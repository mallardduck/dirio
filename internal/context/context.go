package context

type KeyID string

const (
	RequestUserKey KeyID = "requestUser"
	// RequestIDKey is the context key for request IDs
	RequestIDKey KeyID = "requestID"
	// RequestStartTimeKey is the context key for the request start timestamp
	RequestStartTimeKey KeyID = "requestStartTime"
	// TraceIDKey is the context key for trace IDs
	TraceIDKey KeyID = "traceID"
)
