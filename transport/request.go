package transport

import "context"

// RequestKind identifies the Hyperliquid API surface carried by a request.
// Explorer requests are deliberately excluded from WebSocket post requests by
// the protocol, but remain part of the general replacement boundary.
type RequestKind string

const (
	RequestInfo     RequestKind = "info"
	RequestAction   RequestKind = "action"
	RequestExplorer RequestKind = "explorer"
)

// RequestTransport executes one API request and unmarshals its payload into
// response. Implementations must honor ctx. Callers decide retry policy: in
// particular, signed Exchange actions must never be automatically retried.
type RequestTransport interface {
	Request(context.Context, RequestKind, any, any) error
}
