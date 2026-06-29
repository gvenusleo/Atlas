package model

import "context"

// Provider adapts a concrete model backend to Atlas's generic chat protocol.
type Provider interface {
	// Stream executes a streaming model call and returns the accumulated complete response.
	Stream(ctx context.Context, req ChatRequest, emit func(StreamEvent) error) (ChatResponse, error)
}
