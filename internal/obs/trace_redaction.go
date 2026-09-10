package obs

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// redactTraceExporter keeps contextual error text inside the process. Error
// types and status codes remain available for diagnosis without exporting
// amounts or account identifiers embedded in an error's message or stack.
func redactTraceExporter(next sdktrace.SpanExporter) sdktrace.SpanExporter {
	return &traceRedactor{next: next}
}

type traceRedactor struct{ next sdktrace.SpanExporter }

func (r *traceRedactor) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	clean := make([]sdktrace.ReadOnlySpan, len(spans))
	for i, span := range spans {
		clean[i] = redactedSpan{ReadOnlySpan: span}
	}
	return r.next.ExportSpans(ctx, clean)
}

func (r *traceRedactor) Shutdown(ctx context.Context) error { return r.next.Shutdown(ctx) }

type redactedSpan struct{ sdktrace.ReadOnlySpan }

func (s redactedSpan) Attributes() []attribute.KeyValue {
	return scrubTraceAttributes(s.ReadOnlySpan.Attributes())
}

func (s redactedSpan) Events() []sdktrace.Event {
	source := s.ReadOnlySpan.Events()
	clean := make([]sdktrace.Event, len(source))
	for i, event := range source {
		clean[i] = event
		clean[i].Attributes = scrubTraceAttributes(event.Attributes)
	}
	return clean
}

func (s redactedSpan) Links() []sdktrace.Link {
	source := s.ReadOnlySpan.Links()
	clean := make([]sdktrace.Link, len(source))
	for i, link := range source {
		clean[i] = link
		clean[i].Attributes = scrubTraceAttributes(link.Attributes)
	}
	return clean
}

func (s redactedSpan) Status() sdktrace.Status {
	status := s.ReadOnlySpan.Status()
	if status.Description != "" {
		status.Description = "[redacted]"
	}
	return status
}

var traceDenied = append([]string{"user", "loan_id", "loan.id", "account", "customer"}, denied...)

func scrubTraceAttributes(attrs []attribute.KeyValue) []attribute.KeyValue {
	clean := make([]attribute.KeyValue, len(attrs))
	for i, attr := range attrs {
		clean[i] = attr
		key := strings.ToLower(string(attr.Key))
		sensitive := key == "exception.message" || key == "exception.stacktrace" || key == "error.message" || key == "error.stacktrace"
		// Resource and correlation IDs are operational; account and loan IDs are not.
		for _, fragment := range traceDenied {
			if strings.Contains(key, fragment) {
				sensitive = true
				break
			}
		}
		if sensitive {
			clean[i] = attr.Key.String("[redacted]")
			continue
		}
		if attr.Value.Type() == attribute.STRING && len(attr.Value.AsString()) > maxValueBytes {
			clean[i] = attr.Key.String(attr.Value.AsString()[:maxValueBytes] + "…[truncated]")
		}
	}
	return clean
}
