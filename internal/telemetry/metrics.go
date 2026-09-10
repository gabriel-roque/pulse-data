package telemetry

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	Requests            *prometheus.CounterVec
	RequestDuration     *prometheus.HistogramVec
	EventsReceived      *prometheus.CounterVec
	EventsProcessed     *prometheus.CounterVec
	EventsFailed        *prometheus.CounterVec
	EventsDuplicate     *prometheus.CounterVec
	RateLimitRejected   *prometheus.CounterVec
	Published           *prometheus.CounterVec
	WebhookDelivery     *prometheus.CounterVec
	WebhookRetry        *prometheus.CounterVec
	WebhookDLQ          *prometheus.CounterVec
	KafkaPublishLatency *prometheus.HistogramVec
	ProcessingLatency   *prometheus.HistogramVec
	ConsumerLag         *prometheus.GaugeVec
	Workers             *prometheus.GaugeVec
	QueueDepth          *prometheus.GaugeVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		Requests:            prometheus.NewCounterVec(prometheus.CounterOpts{Name: "pulse_http_requests_total", Help: "HTTP requests."}, []string{"method", "route", "status"}),
		RequestDuration:     prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "pulse_http_request_duration_seconds", Help: "HTTP request duration."}, []string{"method", "route"}),
		EventsReceived:      prometheus.NewCounterVec(prometheus.CounterOpts{Name: "pulse_events_received_total", Help: "Events accepted by validation."}, []string{"tenant"}),
		EventsProcessed:     prometheus.NewCounterVec(prometheus.CounterOpts{Name: "pulse_events_processed_total", Help: "Events successfully processed by a worker."}, []string{"worker"}),
		EventsFailed:        prometheus.NewCounterVec(prometheus.CounterOpts{Name: "pulse_events_failed_total", Help: "Event failures."}, []string{"reason"}),
		EventsDuplicate:     prometheus.NewCounterVec(prometheus.CounterOpts{Name: "pulse_events_duplicate_total", Help: "Duplicate events."}, []string{"tenant"}),
		RateLimitRejected:   prometheus.NewCounterVec(prometheus.CounterOpts{Name: "pulse_rate_limit_rejected_total", Help: "Rate limited requests."}, []string{"tenant"}),
		Published:           prometheus.NewCounterVec(prometheus.CounterOpts{Name: "pulse_kafka_published_total", Help: "Published events."}, []string{"topic"}),
		WebhookDelivery:     prometheus.NewCounterVec(prometheus.CounterOpts{Name: "pulse_webhook_delivery_total", Help: "Webhook attempts."}, []string{"result"}),
		WebhookRetry:        prometheus.NewCounterVec(prometheus.CounterOpts{Name: "pulse_webhook_retry_total", Help: "Webhook retries."}, []string{"tenant"}),
		WebhookDLQ:          prometheus.NewCounterVec(prometheus.CounterOpts{Name: "pulse_webhook_dlq_total", Help: "Webhook deliveries moved to DLQ."}, []string{"tenant"}),
		KafkaPublishLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "pulse_kafka_publish_latency_seconds", Help: "Kafka publish latency."}, []string{"topic"}),
		ProcessingLatency:   prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "pulse_processing_latency_seconds", Help: "Worker processing latency."}, []string{"worker"}),
		ConsumerLag:         prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "pulse_consumer_lag", Help: "Consumer lag when reported by the adapter."}, []string{"group", "partition"}),
		Workers:             prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "pulse_workers", Help: "Active workers."}, []string{"worker"}),
		QueueDepth:          prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "pulse_queue_depth", Help: "Bounded in-process queue depth."}, []string{"queue"}),
	}
	for _, c := range []prometheus.Collector{m.Requests, m.RequestDuration, m.EventsReceived, m.EventsProcessed, m.EventsFailed, m.EventsDuplicate, m.RateLimitRejected, m.Published, m.WebhookDelivery, m.WebhookRetry, m.WebhookDLQ, m.KafkaPublishLatency, m.ProcessingLatency, m.ConsumerLag, m.Workers, m.QueueDepth} {
		reg.MustRegister(c)
	}
	return m
}
