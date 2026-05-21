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
	"github.com/horizon6666/max-team/internal/audit"
	"github.com/horizon6666/max-team/internal/config"
	"github.com/horizon6666/max-team/internal/tool"
)

type Request struct {
	Model    string
	System   string
	Messages []anthropic.MessageParam
	Tools    []tool.Tool
	MaxToken int64
}

type ToolExecutor func(ctx context.Context, name string, input json.RawMessage) (string, error)

type Router struct {
	client *anthropic.Client
	config config.LLMConfig
	audit  *audit.Logger
}

func NewRouter(cfg config.LLMConfig, auditLog *audit.Logger) *Router {
	// SDK auto-reads ANTHROPIC_API_KEY, ANTHROPIC_AUTH_TOKEN, ANTHROPIC_BASE_URL from env.
	// Config values override env when explicitly set.
	var opts []option.RequestOption
	if cfg.APIKey != "" {
		opts = append(opts, option.WithAuthToken(cfg.APIKey))
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	client := anthropic.NewClient(opts...)
	return &Router{client: &client, config: cfg, audit: auditLog}
}

func (r *Router) Chat(ctx context.Context, req Request) (*anthropic.Message, error) {
	model := req.Model
	if model == "" {
		model = r.config.DefaultModel
	}
	maxTokens := req.MaxToken
	if maxTokens == 0 {
		maxTokens = 4096
	}

	params := anthropic.MessageNewParams{
		Model:     model,
		Messages:  req.Messages,
		MaxTokens: maxTokens,
	}
	if req.System != "" {
		params.System = []anthropic.TextBlockParam{{Text: req.System}}
	}
	if len(req.Tools) > 0 {
		params.Tools = tool.ToAnthropicTools(req.Tools)
	}

	return r.chatWithRetry(ctx, params)
}

func (r *Router) RunToolLoop(ctx context.Context, req Request, executor ToolExecutor) (string, error) {
	model := req.Model
	if model == "" {
		model = r.config.DefaultModel
	}
	maxTokens := req.MaxToken
	if maxTokens == 0 {
		maxTokens = 4096
	}

	messages := make([]anthropic.MessageParam, len(req.Messages))
	copy(messages, req.Messages)

	var anthropicTools []anthropic.ToolUnionParam
	if len(req.Tools) > 0 {
		anthropicTools = tool.ToAnthropicTools(req.Tools)
	}

	for i := 0; i < 20; i++ {
		params := anthropic.MessageNewParams{
			Model:     model,
			Messages:  messages,
			MaxTokens: maxTokens,
		}
		if req.System != "" {
			params.System = []anthropic.TextBlockParam{{Text: req.System}}
		}
		if len(anthropicTools) > 0 {
			params.Tools = anthropicTools
		}

		resp, err := r.chatWithRetry(ctx, params)
		if err != nil {
			return "", fmt.Errorf("llm call failed: %w", err)
		}

		r.audit.LLMCall("", model, resp.Usage.InputTokens, resp.Usage.OutputTokens)

		messages = append(messages, resp.ToParam())

		if resp.StopReason == "end_turn" || !hasToolUse(resp) {
			return extractText(resp), nil
		}

		toolResults := r.executeTools(ctx, resp, executor)
		messages = append(messages, anthropic.MessageParam{
			Role:    "user",
			Content: toolResults,
		})
	}

	return "", fmt.Errorf("tool loop exceeded max iterations (20)")
}

func (r *Router) executeTools(ctx context.Context, resp *anthropic.Message, executor ToolExecutor) []anthropic.ContentBlockParamUnion {
	var results []anthropic.ContentBlockParamUnion

	for _, block := range resp.Content {
		tb, ok := block.AsAny().(anthropic.ToolUseBlock)
		if !ok {
			continue
		}

		start := time.Now()
		output, err := executor(ctx, tb.Name, tb.Input)
		duration := time.Since(start)

		r.audit.ToolExec("", tb.Name, duration, err)

		if err != nil {
			results = append(results, anthropic.NewToolResultBlock(tb.ID, fmt.Sprintf("Error: %s", err.Error()), true))
		} else {
			results = append(results, anthropic.NewToolResultBlock(tb.ID, output, false))
		}
	}

	return results
}

func (r *Router) chatWithRetry(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
	maxRetries := r.config.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err := r.client.Messages.New(ctx, params)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		if !isRetryable(err) {
			return nil, err
		}

		if attempt < maxRetries {
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

func hasToolUse(msg *anthropic.Message) bool {
	for _, block := range msg.Content {
		if _, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
			return true
		}
	}
	return false
}

func extractText(msg *anthropic.Message) string {
	var text string
	for _, block := range msg.Content {
		if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
			if text != "" {
				text += "\n"
			}
			text += tb.Text
		}
	}
	return text
}

func isRetryable(err error) bool {
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
