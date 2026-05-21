package audit

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/horizon6666/max-team/internal/config"
)

type EventType string

const (
	EventLLMCall    EventType = "llm_call"
	EventToolExec   EventType = "tool_exec"
	EventMsgSent    EventType = "msg_sent"
	EventTaskUpdate EventType = "task_update"
	EventApproval   EventType = "approval"
	EventError      EventType = "error"
)

type Event struct {
	Timestamp time.Time      `json:"ts"`
	Type      EventType      `json:"type"`
	Agent     string         `json:"agent"`
	Data      map[string]any `json:"data"`
}

type Logger struct {
	writer io.Writer
	mu     sync.Mutex
	level  string
}

func New(cfg config.AuditConfig) *Logger {
	if !cfg.Enabled {
		return &Logger{writer: io.Discard, level: "info"}
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Output), 0755); err != nil {
		return &Logger{writer: os.Stderr, level: cfg.Level}
	}
	f, err := os.OpenFile(cfg.Output, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return &Logger{writer: os.Stderr, level: cfg.Level}
	}
	return &Logger{writer: f, level: cfg.Level}
}

func (l *Logger) Record(evt Event) {
	l.mu.Lock()
	defer l.mu.Unlock()
	evt.Timestamp = time.Now()
	data, _ := json.Marshal(evt)
	l.writer.Write(append(data, '\n'))
}

func (l *Logger) LLMCall(agent, model string, inputTokens, outputTokens int64) {
	l.Record(Event{
		Type:  EventLLMCall,
		Agent: agent,
		Data: map[string]any{
			"model":         model,
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		},
	})
}

func (l *Logger) ToolExec(agent, toolName string, duration time.Duration, err error) {
	data := map[string]any{
		"tool":     toolName,
		"duration": duration.String(),
		"success":  err == nil,
	}
	if err != nil {
		data["error"] = err.Error()
	}
	l.Record(Event{Type: EventToolExec, Agent: agent, Data: data})
}

func (l *Logger) Info(eventType EventType, agent string, data map[string]any) {
	l.Record(Event{Type: eventType, Agent: agent, Data: data})
}

func (l *Logger) Error(agent string, msg string, err error) {
	data := map[string]any{"message": msg}
	if err != nil {
		data["error"] = err.Error()
	}
	l.Record(Event{Type: EventError, Agent: agent, Data: data})
}

func (l *Logger) Close() {
	if closer, ok := l.writer.(io.Closer); ok {
		closer.Close()
	}
}
