package kafka

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/pulse-data/pulse/internal/events"
	segmentkafka "github.com/segmentio/kafka-go"
)

type testReader struct {
	messages []segmentkafka.Message
	index    int
	commits  []segmentkafka.Message
	cancel   context.CancelFunc
}

func (r *testReader) FetchMessage(ctx context.Context) (segmentkafka.Message, error) {
	if r.index >= len(r.messages) {
		<-ctx.Done()
		return segmentkafka.Message{}, ctx.Err()
	}
	message := r.messages[r.index]
	r.index++
	return message, nil
}

func (r *testReader) CommitMessages(_ context.Context, messages ...segmentkafka.Message) error {
	r.commits = append(r.commits, messages...)
	r.cancel()
	return nil
}

func (r *testReader) Close() error { return nil }

type flakyWriter struct {
	mu       sync.Mutex
	attempts int
	writes   []segmentkafka.Message
}

func (w *flakyWriter) WriteMessages(_ context.Context, messages ...segmentkafka.Message) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.attempts++
	if w.attempts == 1 {
		return errors.New("temporary DLQ failure")
	}
	w.writes = append(w.writes, messages...)
	return nil
}

func (w *flakyWriter) Close() error { return nil }

func TestBatchRetriesSameMessagesWhenDLQWriteFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reader := &testReader{
		messages: []segmentkafka.Message{
			{Partition: 0, Offset: 1, Value: []byte(`{"tenantId":"tenant","eventId":"event","type":"test","timestamp":"2026-09-11T20:00:00Z","payload":{}}`)},
			{Partition: 0, Offset: 2, Value: []byte(`not-json`)},
		},
		cancel: cancel,
	}
	dlq := &flakyWriter{}
	handled := 0
	consumer := &Consumer{
		reader: reader,
		dlq:    dlq,
		batchHandler: func(_ context.Context, batch []events.Event) error {
			handled += len(batch)
			return nil
		},
		batchSize:    2,
		batchTimeout: time.Millisecond,
	}

	if err := consumer.runBatch(ctx); err != nil {
		t.Fatal(err)
	}
	if dlq.attempts != 2 || len(dlq.writes) != 1 {
		t.Fatalf("DLQ attempts=%d writes=%d, want 2 attempts and 1 write", dlq.attempts, len(dlq.writes))
	}
	if handled != 1 {
		t.Fatalf("handled=%d, want 1", handled)
	}
	if len(reader.commits) != 2 || reader.commits[1].Offset != 2 {
		t.Fatalf("committed messages=%v, want both offsets through 2", reader.commits)
	}
}
