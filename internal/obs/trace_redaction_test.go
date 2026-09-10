package obs

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type recordingTraceExporter struct {
	spans   []sdktrace.ReadOnlySpan
	stopped bool
	err     error
}

func (e *recordingTraceExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.spans = append(e.spans, spans...)
	return e.err
}
func (e *recordingTraceExporter) Shutdown(context.Context) error { e.stopped = true; return e.err }

func TestTraceRedactionRemovesContextualErrors(t *testing.T) {
	ctx := context.Background()
	sink := &recordingTraceExporter{}
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(redactTraceExporter(sink)))
	_, span := provider.Tracer("test").Start(ctx, "plan.search", trace.WithAttributes(
		attribute.String("user_id", "private-account"), attribute.Int64("balance_minor", 123456789), attribute.String("component", "planner"),
	))
	message := "account private-account balance 123456789"
	span.RecordError(errors.New(message), trace.WithStackTrace(true))
	span.AddEvent("exception", trace.WithAttributes(attribute.String("exception.stacktrace", message)))
	span.SetStatus(codes.Error, message)
	span.End()
	if len(sink.spans) != 1 {
		t.Fatalf("exported %d spans", len(sink.spans))
	}
	got := sink.spans[0]
	if got.Name() != "plan.search" || got.Status().Code != codes.Error {
		t.Fatal("safe span metadata lost")
	}
	if got.Status().Description != "[redacted]" {
		t.Fatalf("status = %v", got.Status())
	}
	attrs := got.Attributes()
	if attrs[0].Value.AsString() != "[redacted]" || attrs[1].Value.AsString() != "[redacted]" || attrs[2].Value.AsString() != "planner" {
		t.Fatalf("attributes = %v", attrs)
	}
	sawType := false
	for _, event := range got.Events() {
		for _, attr := range event.Attributes {
			value := attr.Value.String()
			if strings.Contains(value, "private-account") || strings.Contains(value, "123456789") {
				t.Fatalf("private error leaked: %v", attr.Key)
			}
			switch attr.Key {
			case "exception.type":
				sawType = value != "" && value != "[redacted]"
			case "exception.message", "exception.stacktrace":
				if value != "[redacted]" {
					t.Fatalf("error field unredacted: %v", attr.Key)
				}
			}
		}
	}
	if !sawType {
		t.Fatal("exception type lost")
	}
	if err := provider.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if !sink.stopped {
		t.Fatal("shutdown not forwarded")
	}
}

func TestTraceRedactionPreservesOriginalAndExporterErrors(t *testing.T) {
	ctx := context.Background()
	original := &recordingTraceExporter{}
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(original))
	_, span := provider.Tracer("test").Start(ctx, "safe")
	span.RecordError(errors.New("private-account"))
	span.End()
	failure := errors.New("export failed")
	sink := &recordingTraceExporter{err: failure}
	exporter := redactTraceExporter(sink)
	if err := exporter.ExportSpans(ctx, original.spans); !errors.Is(err, failure) {
		t.Fatalf("export error = %v", err)
	}
	for _, attr := range original.spans[0].Events()[0].Attributes {
		if attr.Key == "exception.message" && attr.Value.AsString() != "private-account" {
			t.Fatal("source span was mutated")
		}
	}
	if err := exporter.Shutdown(ctx); !errors.Is(err, failure) {
		t.Fatalf("shutdown error = %v", err)
	}
	if err := provider.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}
