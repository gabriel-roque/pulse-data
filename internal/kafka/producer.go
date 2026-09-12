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
	return p.PublishBatch(ctx, []events.Event{event})
}
func (p *Producer) PublishBatch(ctx context.Context, batch []events.Event) error {
	if len(batch) == 0 {
		return nil
	}
	ctx, span := telemetry.StartSpan(ctx, "pulse.kafka.publish", trace.WithSpanKind(trace.SpanKindProducer), trace.WithAttributes(
		attribute.String("messaging.system", "kafka"),
		attribute.String("messaging.destination.name", p.topic),
		attribute.Int("pulse.batch_size", len(batch)),
	))
	messages := make([]kafka.Message, 0, len(batch))
	for _, event := range batch {
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
		messages = append(messages, kafka.Message{Key: []byte(event.TenantID), Value: b, Headers: headers})
	}
	if err := p.writer.WriteMessages(ctx, messages...); err != nil {
		telemetry.EndSpan(span, err)
		return err
	}
	span.End()
	return nil
}
func (p *Producer) Close() error { return p.writer.Close() }

type ConsumerHandler func(context.Context, events.Event) error
type BatchConsumerHandler func(context.Context, []events.Event) error

type messageReader interface {
	FetchMessage(context.Context) (kafka.Message, error)
	CommitMessages(context.Context, ...kafka.Message) error
	Close() error
}

type messageWriter interface {
	WriteMessages(context.Context, ...kafka.Message) error
	Close() error
}

type Consumer struct {
	reader       messageReader
	handler      ConsumerHandler
	batchHandler BatchConsumerHandler
	batchSize    int
	batchTimeout time.Duration
	dlq          messageWriter
	topic        string
	group        string
	metrics      *telemetry.Metrics
	worker       string
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
	if c.batchHandler != nil && c.batchSize > 1 {
		return c.runBatch(ctx)
	}
	return c.runSingle(ctx)
}

func (c *Consumer) SetBatchHandler(handler BatchConsumerHandler, size int, timeout time.Duration) {
	if size < 1 {
		size = 1
	}
	if timeout <= 0 {
		timeout = 10 * time.Millisecond
	}
	c.batchHandler, c.batchSize, c.batchTimeout = handler, size, timeout
}

func (c *Consumer) runSingle(ctx context.Context) error {
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

func (c *Consumer) runBatch(ctx context.Context) error {
	backoff := time.Second
	for {
		messages, err := c.fetchBatch(ctx)
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

		var processedMessages []kafka.Message
		var batch []events.Event
		for {
			processedMessages, batch, err = c.decodeBatch(ctx, messages)
			if err == nil {
				break
			}
			if ctx.Err() != nil || wait(ctx, backoff) != nil {
				return nil
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
		}
		if len(batch) == 0 {
			if err := c.commitMessages(ctx, processedMessages, backoff); err != nil {
				return nil
			}
			backoff = time.Second
			continue
		}

		started := time.Now()
		for {
			err := c.batchHandler(ctx, batch)
			if err == nil {
				break
			}
			var terminal *BatchDeadLetterError
			if c.dlq != nil && errors.As(err, &terminal) {
				if dlqErr := c.deadLetterBatch(ctx, messages, terminal); dlqErr == nil {
					break
				}
			}
			if wait(ctx, backoff) != nil {
				return nil
			} else if backoff < 30*time.Second {
				backoff *= 2
			}
		}
		if err := c.commitMessages(ctx, processedMessages, backoff); err != nil {
			return nil
		}
		for range batch {
			c.recordProcessing(started, true, nil)
		}
		backoff = time.Second
	}
}

func (c *Consumer) fetchBatch(ctx context.Context) ([]kafka.Message, error) {
	first, err := c.reader.FetchMessage(ctx)
	if err != nil {
		return nil, err
	}
	messages := []kafka.Message{first}
	deadline := time.Now().Add(c.batchTimeout)
	for len(messages) < c.batchSize {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		fetchCtx, cancel := context.WithTimeout(ctx, remaining)
		message, err := c.reader.FetchMessage(fetchCtx)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			break
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func (c *Consumer) decodeBatch(ctx context.Context, messages []kafka.Message) ([]kafka.Message, []events.Event, error) {
	processedMessages := make([]kafka.Message, 0, len(messages))
	batch := make([]events.Event, 0, len(messages))
	for _, message := range messages {
		var event events.Event
		if err := json.Unmarshal(message.Value, &event); err != nil {
			if err := c.writeDeadLetter(ctx, message, fmt.Errorf("decode kafka event: %w", err)); err != nil {
				return nil, nil, err
			}
			processedMessages = append(processedMessages, message)
			continue
		}
		if err := event.Validate(0); err != nil {
			if err := c.writeDeadLetter(ctx, message, fmt.Errorf("validate kafka event: %w", err)); err != nil {
				return nil, nil, err
			}
			processedMessages = append(processedMessages, message)
			continue
		}
		processedMessages = append(processedMessages, message)
		batch = append(batch, event)
	}
	return processedMessages, batch, nil
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
	if err := c.writeDeadLetter(ctx, msg, cause); err != nil {
		return err
	}
	return c.reader.CommitMessages(ctx, msg)
}

func (c *Consumer) writeDeadLetter(ctx context.Context, msg kafka.Message, cause error) error {
	if c.dlq == nil {
		return cause
	}
	headers := append([]kafka.Header(nil), msg.Headers...)
	headers = append(headers, kafka.Header{Key: "pulse-error", Value: []byte(cause.Error())})
	return c.dlq.WriteMessages(ctx, kafka.Message{Key: msg.Key, Value: msg.Value, Headers: headers})
}

func (c *Consumer) deadLetterBatch(ctx context.Context, messages []kafka.Message, terminal *BatchDeadLetterError) error {
	failed := make(map[string]struct{}, len(terminal.Events))
	for _, event := range terminal.Events {
		failed[event.TenantID+"\x00"+event.EventID] = struct{}{}
	}
	if len(failed) == 0 {
		return errors.New("batch dead-letter error contains no events")
	}
	dlqMessages := make([]kafka.Message, 0, len(failed))
	for _, message := range messages {
		var event events.Event
		if err := json.Unmarshal(message.Value, &event); err != nil {
			continue
		}
		if _, ok := failed[event.TenantID+"\x00"+event.EventID]; !ok {
			continue
		}
		headers := append([]kafka.Header(nil), message.Headers...)
		headers = append(headers, kafka.Header{Key: "pulse-error", Value: []byte(terminal.Error())})
		dlqMessages = append(dlqMessages, kafka.Message{Key: message.Key, Value: message.Value, Headers: headers})
		delete(failed, event.TenantID+"\x00"+event.EventID)
	}
	if len(failed) != 0 {
		return fmt.Errorf("batch dead-letter events were not found in kafka batch: %d", len(failed))
	}
	return c.dlq.WriteMessages(ctx, dlqMessages...)
}

func (c *Consumer) commitMessages(ctx context.Context, messages []kafka.Message, backoff time.Duration) error {
	for {
		if err := c.reader.CommitMessages(ctx, messages...); err == nil {
			return nil
		}
		if wait(ctx, backoff) != nil {
			return context.Canceled
		}
	}
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

type BatchDeadLetterError struct {
	Events []events.Event
	Err    error
}

func (e *BatchDeadLetterError) Error() string { return e.Err.Error() }
func (e *BatchDeadLetterError) Unwrap() error { return e.Err }
