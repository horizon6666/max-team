package task

import (
	"fmt"
	"time"
)

type Status int

const (
	StatusCreated Status = iota
	StatusPlanned
	StatusApproved
	StatusDispatched
	StatusRunning
	StatusDone
	StatusFailed
)

func (s Status) String() string {
	switch s {
	case StatusCreated:
		return "created"
	case StatusPlanned:
		return "planned"
	case StatusApproved:
		return "approved"
	case StatusDispatched:
		return "dispatched"
	case StatusRunning:
		return "running"
	case StatusDone:
		return "done"
	case StatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}

var validTransitions = map[Status][]Status{
	StatusCreated:    {StatusPlanned},
	StatusPlanned:    {StatusApproved},
	StatusApproved:   {StatusDispatched},
	StatusDispatched: {StatusRunning},
	StatusRunning:    {StatusDone, StatusFailed},
	StatusFailed:     {StatusDispatched},
}

type Task struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	AgentName   string     `json:"agent_name"`
	DependsOn   []string   `json:"depends_on"`
	Artifacts   []Artifact `json:"artifacts"`
	Status      Status     `json:"status"`
	Result      *Result    `json:"result,omitempty"`
	RetryCount  int        `json:"retry_count"`
	MaxRetries  int        `json:"max_retries"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
}

type Artifact struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
}

type Result struct {
	TaskID    string     `json:"task_id"`
	Success   bool       `json:"success"`
	Output    string     `json:"output"`
	Artifacts []Artifact `json:"artifacts,omitempty"`
	Error     string     `json:"error,omitempty"`
}

func (t *Task) CanTransitionTo(to Status) bool {
	allowed, ok := validTransitions[t.Status]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

func (t *Task) TransitionTo(to Status) error {
	if !t.CanTransitionTo(to) {
		return fmt.Errorf("invalid transition: %s -> %s", t.Status, to)
	}
	t.Status = to
	t.UpdatedAt = time.Now()
	now := time.Now()
	if to == StatusRunning {
		t.StartedAt = &now
	}
	if to == StatusDone || to == StatusFailed {
		t.FinishedAt = &now
	}
	return nil
}
