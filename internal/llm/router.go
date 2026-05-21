package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/horizon6666/max-team/internal/audit"
	"github.com/horizon6666/max-team/internal/config"
	"github.com/horizon6666/max-team/internal/tool"
)

type Request struct {
	Model    string
	Provider string
	System   string
	Messages []Message
	Tools    []tool.Tool
	MaxToken int64
}

type ToolExecutor func(ctx context.Context, name string, input json.RawMessage) (string, error)

type Router struct {
	providers       map[string]Provider
	defaultProvider string
	defaultModel    string
	audit           *audit.Logger
}

func NewRouter(cfg config.LLMConfig, auditLog *audit.Logger) *Router {
	r := &Router{
		providers:       make(map[string]Provider),
		defaultProvider: cfg.DefaultProvider,
		defaultModel:    cfg.DefaultModel,
		audit:           auditLog,
	}

	for name, pcfg := range cfg.Providers {
		switch name {
		case "anthropic":
			r.providers[name] = NewAnthropicProvider(pcfg)
		case "openai":
			r.providers[name] = NewOpenAIProvider(pcfg)
		default:
			log.Printf("[router] unknown provider: %s, skipping", name)
		}
	}

	if len(r.providers) == 0 && cfg.APIKey != "" {
		pcfg := config.ProviderConfig{
			APIKey:     cfg.APIKey,
			BaseURL:    cfg.BaseURL,
			MaxRetries: cfg.MaxRetries,
		}
		providerName := cfg.DefaultProvider
		if providerName == "" {
			providerName = "anthropic"
		}
		switch providerName {
		case "openai":
			r.providers[providerName] = NewOpenAIProvider(pcfg)
		default:
			r.providers[providerName] = NewAnthropicProvider(pcfg)
		}
		r.defaultProvider = providerName
	}

	if r.defaultProvider == "" && len(r.providers) > 0 {
		for name := range r.providers {
			r.defaultProvider = name
			break
		}
	}

	return r
}

func (r *Router) RunToolLoop(ctx context.Context, req Request, executor ToolExecutor) (string, error) {
	provider := r.getProvider(req.Provider)
	if provider == nil {
		return "", fmt.Errorf("provider not found: %s (default: %s)", req.Provider, r.defaultProvider)
	}

	model := req.Model
	if model == "" {
		model = r.defaultModel
	}
	maxToken := req.MaxToken
	if maxToken == 0 {
		maxToken = 4096
	}

	toolDefs := toToolDefs(req.Tools)
	messages := make([]Message, len(req.Messages))
	copy(messages, req.Messages)

	for i := 0; i < 20; i++ {
		pReq := ProviderRequest{
			Model:    model,
			System:   req.System,
			Messages: messages,
			Tools:    toolDefs,
			MaxToken: maxToken,
		}

		resp, err := provider.Chat(ctx, pReq)
		if err != nil {
			return "", fmt.Errorf("llm call failed: %w", err)
		}

		r.audit.LLMCall("", model, resp.InputTokens, resp.OutputTokens)

		messages = append(messages, Message{
			Role:      RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		if resp.StopReason == "end_turn" || len(resp.ToolCalls) == 0 {
			return resp.Content, nil
		}

		var results []ToolResult
		for _, tc := range resp.ToolCalls {
			start := time.Now()
			output, err := executor(ctx, tc.Name, tc.Input)
			duration := time.Since(start)
			r.audit.ToolExec("", tc.Name, duration, err)

			if err != nil {
				results = append(results, ToolResult{
					ToolCallID: tc.ID,
					Content:    fmt.Sprintf("Error: %s", err.Error()),
					IsError:    true,
				})
			} else {
				results = append(results, ToolResult{
					ToolCallID: tc.ID,
					Content:    output,
				})
			}
		}

		messages = append(messages, Message{
			Role:        RoleUser,
			ToolResults: results,
		})
	}

	return "", fmt.Errorf("tool loop exceeded max iterations (20)")
}

func (r *Router) getProvider(name string) Provider {
	if name != "" {
		if p, ok := r.providers[name]; ok {
			return p
		}
	}
	return r.providers[r.defaultProvider]
}

func toToolDefs(tools []tool.Tool) []ToolDef {
	defs := make([]ToolDef, len(tools))
	for i, t := range tools {
		defs[i] = ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: t.InputSchema(),
		}
	}
	return defs
}
