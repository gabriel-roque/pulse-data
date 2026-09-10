package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/pulse-data/pulse/internal/events"
	"github.com/pulse-data/pulse/internal/telemetry"
	"github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type Publisher interface {
	Publish(context.Context, events.Event) error
}

type Producer struct {
	writer  *kafka.Writer
	topic   string
	brokers []string
}

func NewProducer(brokers []string, topic string) *Producer {
	return &Producer{writer: &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireAll,
		Async:        false,
		BatchSize:    100,
		BatchTimeout: 5 * time.Millisecond,
	}, topic: topic, brokers: brokers}
}

func (p *Producer) Ready(ctx context.Context) error {
	if len(p.brokers) == 0 {
		return errors.New("kafka brokers are not configured")
	}
	dialer := &kafka.Dialer{Timeout: 3 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", p.brokers[0])
	if err != nil {
		return err
	}
	return conn.Close()
}
func (p *Producer) Publish(ctx context.Context, event events.Event) error {
	ctx, span := telemetry.StartSpan(ctx, "pulse.kafka.publish", trace.WithSpanKind(trace.SpanKindProducer), trace.WithAttributes(
		attribute.String("messaging.system", "kafka"),
		attribute.String("messaging.destination.name", p.topic),
	))
	b, err := json.Marshal(event)
	if err != nil {
		telemetry.EndSpan(span, err)
		return err
	}
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	headers := make([]kafka.Header, 0, len(carrier)+1)
	for key, value := range carrier {
		headers = append(headers, kafka.Header{Key: key, Value: []byte(value)})
	}
	headers = append(headers, kafka.Header{Key: "event-id", Value: []byte(event.EventID)})
	if err := p.writer.WriteMessages(ctx, kafka.Message{Key: []byte(event.TenantID), Value: b, Headers: headers}); err != nil {
		telemetry.EndSpan(span, err)
		return err
	}
	span.End()
	return nil
}
func (p *Producer) Close() error { return p.writer.Close() }

type ConsumerHandler func(context.Context, events.Event) error
type Consumer struct {
	reader  *kafka.Reader
	handler ConsumerHandler
	dlq     *kafka.Writer
	topic   string
	group   string
	metrics *telemetry.Metrics
	worker  string
}

func NewConsumer(brokers []string, topic, group string, handler ConsumerHandler) *Consumer {
	return &Consumer{reader: kafka.NewReader(kafka.ReaderConfig{Brokers: brokers, Topic: topic, GroupID: group, MinBytes: 1, MaxBytes: 10 << 20}), handler: handler, topic: topic, group: group}
}
func NewConsumerWithDLQ(brokers []string, topic, group, dlqTopic string, handler ConsumerHandler) *Consumer {
	c := NewConsumer(brokers, topic, group, handler)
	c.dlq = &kafka.Writer{Addr: kafka.TCP(brokers...), Topic: dlqTopic, Balancer: &kafka.Hash{}, RequiredAcks: kafka.RequireAll, Async: false}
	return c
}

func (c *Consumer) SetMetrics(metrics *telemetry.Metrics, worker string) {
	c.metrics, c.worker = metrics, worker
}

func (c *Consumer) Run(ctx context.Context) error {
	backoff := time.Second
	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if err := wait(ctx, backoff); err != nil {
				return nil
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		carrier := propagation.MapCarrier{}
		for _, header := range msg.Headers {
			carrier[header.Key] = string(header.Value)
		}
		messageCtx := propagation.TraceContext{}.Extract(ctx, carrier)
		messageCtx, span := telemetry.StartSpan(messageCtx, "pulse.kafka.process", trace.WithSpanKind(trace.SpanKindConsumer), trace.WithAttributes(
			attribute.String("messaging.system", "kafka"),
			attribute.String("messaging.destination.name", c.topic),
			attribute.String("messaging.consumer.group", c.group),
		))
		started := time.Now()
		processed, processErr := c.processMessage(messageCtx, msg)
		if processErr != nil {
			telemetry.EndSpan(span, processErr)
		} else {
			span.End()
		}
		c.recordProcessing(started, processed, processErr)
		if processErr != nil {
			if err := wait(ctx, backoff); err != nil {
				return nil
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
	}
}

func (c *Consumer) processMessage(ctx context.Context, msg kafka.Message) (bool, error) {
	var event events.Event
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		return false, c.deadLetter(ctx, msg, fmt.Errorf("decode kafka event: %w", err))
	}
	if err := event.Validate(0); err != nil {
		return false, c.deadLetter(ctx, msg, fmt.Errorf("validate kafka event: %w", err))
	}
	if err := c.handler(ctx, event); err != nil {
		var terminal *DeadLetterError
		if c.dlq != nil && errors.As(err, &terminal) {
			if dlqErr := c.dlq.WriteMessages(ctx, kafka.Message{Key: msg.Key, Value: msg.Value, Headers: append(msg.Headers, kafka.Header{Key: "pulse-error", Value: []byte(err.Error())})}); dlqErr != nil {
				return false, dlqErr
			}
			if commitErr := c.reader.CommitMessages(ctx, msg); commitErr != nil {
				return false, commitErr
			}
			return false, nil
		}
		return false, err
	}
	if err := c.reader.CommitMessages(ctx, msg); err != nil {
		return false, err
	}
	return true, nil
}

func (c *Consumer) deadLetter(ctx context.Context, msg kafka.Message, cause error) error {
	if c.dlq == nil {
		return cause
	}
	if err := c.dlq.WriteMessages(ctx, kafka.Message{Key: msg.Key, Value: msg.Value, Headers: append(msg.Headers, kafka.Header{Key: "pulse-error", Value: []byte(cause.Error())})}); err != nil {
		return err
	}
	return c.reader.CommitMessages(ctx, msg)
}

func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Consumer) recordProcessing(started time.Time, processed bool, err error) {
	if c.metrics == nil {
		return
	}
	worker := c.worker
	if worker == "" {
		worker = c.group
	}
	c.metrics.ProcessingLatency.WithLabelValues(worker).Observe(time.Since(started).Seconds())
	if processed {
		c.metrics.EventsProcessed.WithLabelValues(worker).Inc()
	}
	if err != nil {
		c.metrics.EventsFailed.WithLabelValues("processing").Inc()
	}
}
func (c *Consumer) Close() error {
	if c.dlq != nil {
		_ = c.dlq.Close()
	}
	return c.reader.Close()
}

type DeadLetterError struct{ Err error }

func (e *DeadLetterError) Error() string { return e.Err.Error() }
func (e *DeadLetterError) Unwrap() error { return e.Err }
