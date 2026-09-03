package obs

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	requestMeter       = otel.Meter("github.com/andranikasd/marumbot/requests")
	webhookDuration, _ = requestMeter.Float64Histogram("marum_webhook_duration_seconds", metric.WithUnit("s"), metric.WithDescription("webhook acceptance and inline command duration"))
	queueDepth, _      = requestMeter.Int64Gauge("marum_queue_depth", metric.WithDescription("pending work at the last successful tick or status probe"))
	queueAge, _        = requestMeter.Int64Gauge("marum_queue_oldest_age_seconds", metric.WithUnit("s"))
)

// RecordWebhook uses a fixed set of outcomes, never a URL or account label.
func RecordWebhook(ctx context.Context, elapsed time.Duration, status int) {
	outcome := "error"
	if status < 400 {
		outcome = "accepted"
	} else if status < 500 {
		outcome = "rejected"
	}
	webhookDuration.Record(ctx, elapsed.Seconds(), metric.WithAttributes(attribute.String("outcome", outcome)))
}

// RecordQueues records only aggregate counts and ages. A failed probe must not
// call this with zeroes: an unavailable queue is different from an empty queue.
func RecordQueues(ctx context.Context, commands, deliveries, commandAge, deliveryAge int64) {
	command := metric.WithAttributes(attribute.String("queue", "commands"))
	delivery := metric.WithAttributes(attribute.String("queue", "deliveries"))
	queueDepth.Record(ctx, commands, command)
	queueDepth.Record(ctx, deliveries, delivery)
	queueAge.Record(ctx, commandAge, command)
	queueAge.Record(ctx, deliveryAge, delivery)
}

var (
	telegramDuration, _ = requestMeter.Float64Histogram("marum_telegram_call_duration_seconds", metric.WithUnit("s"), metric.WithDescription("Telegram call duration including local pacing"))
	telegramLimited, _  = requestMeter.Int64Counter("marum_telegram_rate_limited_total", metric.WithDescription("Telegram calls that received HTTP 429"))
)

// RecordTelegram receives the sender adapter's closed method/outcome vocabulary.
func RecordTelegram(ctx context.Context, method, outcome string, elapsed time.Duration, limited bool) {
	labels := metric.WithAttributes(attribute.String("method", method), attribute.String("outcome", outcome))
	telegramDuration.Record(ctx, elapsed.Seconds(), labels)
	if limited {
		telegramLimited.Add(ctx, 1, metric.WithAttributes(attribute.String("method", method)))
	}
}
