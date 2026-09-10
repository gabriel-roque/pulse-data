package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/pulse-data/pulse/internal/events"
	"github.com/segmentio/kafka-go"
)

type Publisher interface {
	Publish(context.Context, events.Event) error
}

type Producer struct {
	writer *kafka.Writer
	topic  string
}

func NewProducer(brokers []string, topic string) *Producer {
	return &Producer{writer: &kafka.Writer{Addr: kafka.TCP(brokers...), Topic: topic, Balancer: &kafka.Hash{}, RequiredAcks: kafka.RequireAll, Async: false}, topic: topic}
}
func (p *Producer) Publish(ctx context.Context, event events.Event) error {
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return p.writer.WriteMessages(ctx, kafka.Message{Key: []byte(event.TenantID), Value: b, Headers: []kafka.Header{{Key: "event-id", Value: []byte(event.EventID)}}})
}
func (p *Producer) Close() error { return p.writer.Close() }

type ConsumerHandler func(context.Context, events.Event) error
type Consumer struct {
	reader  *kafka.Reader
	handler ConsumerHandler
	dlq     *kafka.Writer
}

func NewConsumer(brokers []string, topic, group string, handler ConsumerHandler) *Consumer {
	return &Consumer{reader: kafka.NewReader(kafka.ReaderConfig{Brokers: brokers, Topic: topic, GroupID: group, MinBytes: 1, MaxBytes: 10 << 20}), handler: handler}
}
func NewConsumerWithDLQ(brokers []string, topic, group, dlqTopic string, handler ConsumerHandler) *Consumer {
	c := NewConsumer(brokers, topic, group, handler)
	c.dlq = &kafka.Writer{Addr: kafka.TCP(brokers...), Topic: dlqTopic, Balancer: &kafka.Hash{}, RequiredAcks: kafka.RequireAll, Async: false}
	return c
}
func (c *Consumer) Run(ctx context.Context) error {
	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		var event events.Event
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			if c.dlq == nil {
				return fmt.Errorf("decode kafka event: %w", err)
			}
			if dlqErr := c.dlq.WriteMessages(ctx, kafka.Message{Key: msg.Key, Value: msg.Value, Headers: append(msg.Headers, kafka.Header{Key: "pulse-error", Value: []byte(err.Error())})}); dlqErr != nil {
				return dlqErr
			}
			if commitErr := c.reader.CommitMessages(ctx, msg); commitErr != nil {
				return commitErr
			}
			continue
		}
		if err := c.handler(ctx, event); err != nil {
			var terminal *DeadLetterError
			if c.dlq != nil && errors.As(err, &terminal) {
				if dlqErr := c.dlq.WriteMessages(ctx, kafka.Message{Key: msg.Key, Value: msg.Value, Headers: append(msg.Headers, kafka.Header{Key: "pulse-error", Value: []byte(err.Error())})}); dlqErr != nil {
					return dlqErr
				}
				if commitErr := c.reader.CommitMessages(ctx, msg); commitErr != nil {
					return commitErr
				}
				continue
			}
			return err
		}
		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			return err
		}
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
