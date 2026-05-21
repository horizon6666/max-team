package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/horizon6666/max-team/internal/config"
)

type AnthropicProvider struct {
	client     *anthropic.Client
	maxRetries int
}

func NewAnthropicProvider(cfg config.ProviderConfig) *AnthropicProvider {
	var opts []option.RequestOption
	if cfg.APIKey != "" {
		opts = append(opts, option.WithAuthToken(cfg.APIKey))
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	client := anthropic.NewClient(opts...)
	maxRetries := cfg.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}
	return &AnthropicProvider{client: &client, maxRetries: maxRetries}
}

func (p *AnthropicProvider) Chat(ctx context.Context, req ProviderRequest) (*Response, error) {
	messages := toAnthropicMessages(req.Messages)

	params := anthropic.MessageNewParams{
		Model:     req.Model,
		Messages:  messages,
		MaxTokens: req.MaxToken,
	}
	if req.System != "" {
		params.System = []anthropic.TextBlockParam{{Text: req.System}}
	}
	if len(req.Tools) > 0 {
		params.Tools = toAnthropicTools(req.Tools)
	}

	resp, err := p.chatWithRetry(ctx, params)
	if err != nil {
		return nil, err
	}

	return fromAnthropicResponse(resp), nil
}

func (p *AnthropicProvider) chatWithRetry(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
	var lastErr error
	for attempt := 0; attempt <= p.maxRetries; attempt++ {
		resp, err := p.client.Messages.New(ctx, params)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !isAnthropicRetryable(err) {
			return nil, err
		}
		if attempt < p.maxRetries {
			backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
	}
	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

func toAnthropicMessages(msgs []Message) []anthropic.MessageParam {
	var out []anthropic.MessageParam
	for _, m := range msgs {
		switch m.Role {
		case RoleUser:
			if len(m.ToolResults) > 0 {
				var blocks []anthropic.ContentBlockParamUnion
				for _, tr := range m.ToolResults {
					blocks = append(blocks, anthropic.NewToolResultBlock(tr.ToolCallID, tr.Content, tr.IsError))
				}
				out = append(out, anthropic.MessageParam{Role: "user", Content: blocks})
			} else {
				out = append(out, anthropic.MessageParam{
					Role:    "user",
					Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock(m.Content)},
				})
			}
		case RoleAssistant:
			if len(m.ToolCalls) > 0 {
				var blocks []anthropic.ContentBlockParamUnion
				if m.Content != "" {
					blocks = append(blocks, anthropic.NewTextBlock(m.Content))
				}
				for _, tc := range m.ToolCalls {
					blocks = append(blocks, anthropic.ContentBlockParamUnion{
						OfToolUse: &anthropic.ToolUseBlockParam{
							ID:    tc.ID,
							Name:  tc.Name,
							Input: tc.Input,
						},
					})
				}
				out = append(out, anthropic.MessageParam{Role: "assistant", Content: blocks})
			} else {
				out = append(out, anthropic.MessageParam{
					Role:    "assistant",
					Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock(m.Content)},
				})
			}
		}
	}
	return out
}

func toAnthropicTools(tools []ToolDef) []anthropic.ToolUnionParam {
	params := make([]anthropic.ToolUnionParam, len(tools))
	for i, t := range tools {
		var schema anthropic.ToolInputSchemaParam
		json.Unmarshal(t.InputSchema, &schema)
		tp := anthropic.ToolParam{
			Name:        t.Name,
			Description: anthropic.String(t.Description),
			InputSchema: schema,
		}
		params[i] = anthropic.ToolUnionParam{OfTool: &tp}
	}
	return params
}

func fromAnthropicResponse(msg *anthropic.Message) *Response {
	resp := &Response{
		StopReason:   string(msg.StopReason),
		InputTokens:  msg.Usage.InputTokens,
		OutputTokens: msg.Usage.OutputTokens,
	}
	for _, block := range msg.Content {
		switch v := block.AsAny().(type) {
		case anthropic.TextBlock:
			if resp.Content != "" {
				resp.Content += "\n"
			}
			resp.Content += v.Text
		case anthropic.ToolUseBlock:
			resp.ToolCalls = append(resp.ToolCalls, ToolCall{
				ID:    v.ID,
				Name:  v.Name,
				Input: v.Input,
			})
		}
	}
	return resp
}

func isAnthropicRetryable(err error) bool {
	var apiErr *anthropic.Error
	if ok := errors.As(err, &apiErr); ok {
		code := apiErr.StatusCode
		return code == http.StatusTooManyRequests ||
			code == http.StatusInternalServerError ||
			code == http.StatusBadGateway ||
			code == http.StatusServiceUnavailable
	}
	return false
}
