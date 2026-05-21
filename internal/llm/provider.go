package llm

import "context"

type ProviderRequest struct {
	Model    string
	System   string
	Messages []Message
	Tools    []ToolDef
	MaxToken int64
}

type Provider interface {
	Chat(ctx context.Context, req ProviderRequest) (*Response, error)
}

type Response struct {
	Content      string
	ToolCalls    []ToolCall
	StopReason   string
	InputTokens  int64
	OutputTokens int64
}
