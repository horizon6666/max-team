package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/shared"

	"github.com/horizon6666/max-team/internal/config"
)

type OpenAIProvider struct {
	client     *openai.Client
	maxRetries int
}

func NewOpenAIProvider(cfg config.ProviderConfig) *OpenAIProvider {
	var opts []option.RequestOption
	if cfg.APIKey != "" {
		opts = append(opts, option.WithAPIKey(cfg.APIKey))
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	client := openai.NewClient(opts...)
	maxRetries := cfg.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}
	return &OpenAIProvider{client: &client, maxRetries: maxRetries}
}

func (p *OpenAIProvider) Chat(ctx context.Context, req ProviderRequest) (*Response, error) {
	messages := toOpenAIMessages(req.System, req.Messages)

	params := openai.ChatCompletionNewParams{
		Model:    req.Model,
		Messages: messages,
	}
	if req.MaxToken > 0 {
		params.MaxTokens = param.NewOpt(req.MaxToken)
	}
	if len(req.Tools) > 0 {
		params.Tools = toOpenAITools(req.Tools)
	}

	resp, err := p.chatWithRetry(ctx, params)
	if err != nil {
		return nil, err
	}

	return fromOpenAIResponse(resp), nil
}

func (p *OpenAIProvider) chatWithRetry(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	var lastErr error
	for attempt := 0; attempt <= p.maxRetries; attempt++ {
		resp, err := p.client.Chat.Completions.New(ctx, params)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !isOpenAIRetryable(err) {
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

func toOpenAIMessages(system string, msgs []Message) []openai.ChatCompletionMessageParamUnion {
	var out []openai.ChatCompletionMessageParamUnion

	if system != "" {
		out = append(out, openai.SystemMessage(system))
	}

	for _, m := range msgs {
		switch m.Role {
		case RoleUser:
			if len(m.ToolResults) > 0 {
				for _, tr := range m.ToolResults {
					out = append(out, openai.ToolMessage(tr.Content, tr.ToolCallID))
				}
			} else {
				out = append(out, openai.UserMessage(m.Content))
			}
		case RoleAssistant:
			if len(m.ToolCalls) > 0 {
				asst := openai.ChatCompletionAssistantMessageParam{}
				if m.Content != "" {
					asst.Content.OfString = param.NewOpt(m.Content)
				}
				asst.ToolCalls = make([]openai.ChatCompletionMessageToolCallParam, len(m.ToolCalls))
				for i, tc := range m.ToolCalls {
					asst.ToolCalls[i] = openai.ChatCompletionMessageToolCallParam{
						ID: tc.ID,
						Function: openai.ChatCompletionMessageToolCallFunctionParam{
							Name:      tc.Name,
							Arguments: string(tc.Input),
						},
					}
				}
				out = append(out, openai.ChatCompletionMessageParamUnion{OfAssistant: &asst})
			} else {
				out = append(out, openai.AssistantMessage(m.Content))
			}
		}
	}
	return out
}

func toOpenAITools(tools []ToolDef) []openai.ChatCompletionToolParam {
	params := make([]openai.ChatCompletionToolParam, len(tools))
	for i, t := range tools {
		var schema shared.FunctionParameters
		json.Unmarshal(t.InputSchema, &schema)
		params[i] = openai.ChatCompletionToolParam{
			Function: shared.FunctionDefinitionParam{
				Name:        t.Name,
				Description: param.NewOpt(t.Description),
				Parameters:  schema,
			},
		}
	}
	return params
}

func fromOpenAIResponse(resp *openai.ChatCompletion) *Response {
	if len(resp.Choices) == 0 {
		return &Response{StopReason: "end_turn"}
	}

	choice := resp.Choices[0]
	result := &Response{
		Content:      choice.Message.Content,
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
	}

	if choice.FinishReason == "tool_calls" {
		result.StopReason = "tool_use"
	} else {
		result.StopReason = "end_turn"
	}

	for _, tc := range choice.Message.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, ToolCall{
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: json.RawMessage(tc.Function.Arguments),
		})
	}

	return result
}

func isOpenAIRetryable(err error) bool {
	type httpError interface {
		StatusCode() int
	}
	if he, ok := err.(httpError); ok {
		code := he.StatusCode()
		return code == http.StatusTooManyRequests ||
			code == http.StatusInternalServerError ||
			code == http.StatusBadGateway ||
			code == http.StatusServiceUnavailable
	}
	return false
}
