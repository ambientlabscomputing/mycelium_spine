package sdk

import "context"

// traceIDKey is an unexported type for context keys to avoid collisions.
type traceIDKey struct{}

// WithTraceID returns a new context with the trace ID attached.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

// TraceIDFromContext extracts the trace ID from the context.
// Returns an empty string if no trace ID is present.
func TraceIDFromContext(ctx context.Context) string {
	if traceID, ok := ctx.Value(traceIDKey{}).(string); ok {
		return traceID
	}
	return ""
}
