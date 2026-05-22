package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/horizon6666/max-team/internal/audit"
	"github.com/horizon6666/max-team/internal/bus"
	"github.com/horizon6666/max-team/internal/config"
	"github.com/horizon6666/max-team/internal/llm"
	"github.com/horizon6666/max-team/internal/tool"
)

type AgentStatus struct {
	Name        string `json:"name"`
	Role        string `json:"role"`
	Status      string `json:"status"`
	CurrentTask string `json:"current_task"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
}

type Agent interface {
	Name() string
	Start(ctx context.Context) error
	Run(ctx context.Context)
	Stop()
	Status() AgentStatus
}

type BaseAgent struct {
	config      config.AgentConfig
	router      *llm.Router
	tools       []tool.Tool
	bus         *bus.MessageBus
	inbox       <-chan bus.Message
	audit       *audit.Logger
	history     []llm.Message
	mu          sync.RWMutex
	status      string
	currentTask string
}

func NewBaseAgent(cfg config.AgentConfig, r *llm.Router, tools []tool.Tool, b *bus.MessageBus, a *audit.Logger) BaseAgent {
	inbox := b.Subscribe(cfg.Name)
	return BaseAgent{
		config: cfg,
		router: r,
		tools:  tools,
		bus:    b,
		inbox:  inbox,
		audit:  a,
		status: "idle",
	}
}

func (b *BaseAgent) Name() string { return b.config.Name }

func (b *BaseAgent) Start(_ context.Context) error {
	log.Printf("[%s] agent started (model=%s, tools=%d)", b.config.Name, b.config.Model, len(b.tools))
	return nil
}

func (b *BaseAgent) Stop() {
	log.Printf("[%s] agent stopped", b.config.Name)
}

func (b *BaseAgent) Status() AgentStatus {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return AgentStatus{
		Name:        b.config.Name,
		Role:        b.config.Role,
		Status:      b.status,
		CurrentTask: b.currentTask,
		Provider:    b.config.Provider,
		Model:       b.config.Model,
	}
}

func (b *BaseAgent) setStatus(status, currentTask string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.status = status
	b.currentTask = currentTask
}

const maxHistoryLen = 20

func (b *BaseAgent) ResetHistory() {
	b.history = nil
}

func (b *BaseAgent) trimHistory() {
	if len(b.history) > maxHistoryLen {
		b.history = b.history[len(b.history)-maxHistoryLen:]
	}
}

func (b *BaseAgent) RunLLM(ctx context.Context, userMessage string) (string, error) {
	b.mu.Lock()
	b.status = "thinking"
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		b.status = "idle"
		b.mu.Unlock()
	}()

	b.history = append(b.history, llm.Message{
		Role:    llm.RoleUser,
		Content: userMessage,
	})

	executor := func(ctx context.Context, name string, input json.RawMessage) (string, error) {
		for _, t := range b.tools {
			if t.Name() == name {
				return t.Execute(ctx, input)
			}
		}
		return "", fmt.Errorf("tool not found: %s", name)
	}

	req := llm.Request{
		Model:    b.config.Model,
		Provider: b.config.Provider,
		System:   b.config.SystemPrompt,
		Messages: b.history,
		Tools:    b.tools,
		MaxToken: int64(b.config.Constraints.MaxTokens),
	}

	result, err := b.router.RunToolLoop(ctx, req, executor)
	if err != nil {
		return "", err
	}

	b.history = append(b.history, llm.Message{
		Role:    llm.RoleAssistant,
		Content: result,
	})
	b.trimHistory()

	return result, nil
}

func (b *BaseAgent) Send(to string, msgType bus.MessageType, payload any) {
	b.bus.Send(bus.Message{
		From:    b.config.Name,
		To:      to,
		Type:    msgType,
		Payload: payload,
	})
}

func (b *BaseAgent) Reply(original bus.Message, msgType bus.MessageType, payload any) {
	b.bus.Send(bus.Message{
		From:    b.config.Name,
		To:      original.From,
		Type:    msgType,
		Payload: payload,
		ReplyTo: original.ID,
	})
}
