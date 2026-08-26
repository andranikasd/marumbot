package obs

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Component names a logical process inside the binary.
//
// Marum is one deployable but several independent pieces of work: a webhook
// that must answer in milliseconds, a worker that applies effects, a scheduler
// that ticks, a sender that is rate limited, an engine that only computes.
// A service graph keyed on service.name collapses all of that into one node,
// which is true of the deployment and useless for understanding the system.
//
// Every span carries the component it belongs to, so latency, errors and
// throughput can be read per piece, and so a trace shows which part of the
// monolith a request actually spent its time in.
type Component string

const (
	ComponentWebhook   Component = "webhook"   // authenticate, normalise, persist, answer
	ComponentWorker    Component = "worker"    // lease a command and apply its effect
	ComponentScheduler Component = "scheduler" // tick: generate, group, reconcile
	ComponentSender    Component = "sender"    // the single rate-limited Telegram egress
	ComponentEngine    Component = "engine"    // pure calculation
	ComponentStore     Component = "store"     // database access
	ComponentAdmin     Component = "admin"     // the private operator interface
)

const componentKey = attribute.Key("marum.component")

// Attr renders the component as a span attribute and a metric label. The set
// is closed, so it is safe as a label.
func (c Component) Attr() attribute.KeyValue { return componentKey.String(string(c)) }

// Service is the name this component reports as.
//
// A service graph draws nodes from service.name and edges from client/server
// span pairs. With one name for the whole binary the graph is a single node,
// which is true of the deployment and says nothing about the system. Each
// component therefore reports as its own service, grouped by a shared
// service.namespace, so the graph shows the pieces and the flow between them.
func (c Component) Service() string { return "marum-" + string(c) }

// tracers is populated by Init: one provider per component, each with its own
// service.name. Before Init, and in tests, calls fall back to the global
// provider and simply produce ordinary spans.
var tracers = map[Component]trace.Tracer{}

func (c Component) tracer() trace.Tracer {
	if t, ok := tracers[c]; ok {
		return t
	}
	return otel.Tracer("marum/" + string(c))
}

// Enter opens the callee side of a boundary: a SERVER span belonging to this
// component. Pair it with Call on the caller and the graph gains an edge.
func (c Component) Enter(ctx context.Context, operation string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	opts = append(opts,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(c.Attr()))
	return c.tracer().Start(ctx, string(c)+"."+operation, opts...)
}

// Call opens the caller side of a boundary: a CLIENT span in this component
// naming the component it is about to call.
//
// peer.service is what lets the graph draw the edge even when the callee is
// not instrumented, so an uninstrumented dependency appears as a node rather
// than as an edge into nothing.
func (c Component) Call(ctx context.Context, target Component, operation string) (context.Context, trace.Span) {
	return c.CallService(ctx, target.Service(), string(target)+"."+operation)
}

// CallService is Call for a peer that is not one of ours - a database, or
// Telegram - so an external dependency is a node rather than a dead end.
func (c Component) CallService(ctx context.Context, peer, operation string) (context.Context, trace.Span) {
	return c.tracer().Start(ctx, string(c)+"→"+operation,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			c.Attr(),
			attribute.String("peer.service", peer),
		))
}

// Start opens an internal span: work inside one component, not a boundary.
func (c Component) Start(ctx context.Context, operation string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	opts = append(opts, trace.WithAttributes(c.Attr()))
	return c.tracer().Start(ctx, string(c)+"."+operation, opts...)
}

// All is every component, so Init can build a provider for each.
func All() []Component {
	return []Component{
		ComponentWebhook, ComponentWorker, ComponentScheduler,
		ComponentSender, ComponentEngine, ComponentStore, ComponentAdmin,
	}
}

// From reports the component of the active span, or an empty Component when
// there is none. Used by the log enricher so a log line says which part of the
// system produced it.
func From(ctx context.Context) Component {
	if s := trace.SpanFromContext(ctx); s.SpanContext().IsValid() {
		if rw, ok := s.(interface{ Attributes() []attribute.KeyValue }); ok {
			for _, kv := range rw.Attributes() {
				if kv.Key == componentKey {
					return Component(kv.Value.AsString())
				}
			}
		}
	}
	return ""
}
