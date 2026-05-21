package task

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Manager struct {
	mu    sync.RWMutex
	tasks map[string]*Task
	order []string
}

func NewManager() *Manager {
	return &Manager{
		tasks: make(map[string]*Task),
	}
}

func (m *Manager) Create(t *Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t.ID == "" {
		t.ID = uuid.New().String()[:8]
	}
	t.Status = StatusCreated
	t.CreatedAt = time.Now()
	t.UpdatedAt = time.Now()
	if t.MaxRetries == 0 {
		t.MaxRetries = 2
	}
	m.tasks[t.ID] = t
	m.order = append(m.order, t.ID)
	return nil
}

func (m *Manager) Get(taskID string) *Task {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tasks[taskID]
}

func (m *Manager) Transition(taskID string, to Status) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[taskID]
	if !ok {
		return fmt.Errorf("task %s not found", taskID)
	}
	return t.TransitionTo(to)
}

func (m *Manager) SetResult(taskID string, result *Result) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[taskID]
	if !ok {
		return fmt.Errorf("task %s not found", taskID)
	}
	t.Result = result
	return nil
}

func (m *Manager) ReadyToDispatch() []*Task {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var ready []*Task
	for _, id := range m.order {
		t := m.tasks[id]
		if t.Status != StatusApproved {
			continue
		}
		allDone := true
		for _, depID := range t.DependsOn {
			dep := m.tasks[depID]
			if dep == nil || dep.Status != StatusDone {
				allDone = false
				break
			}
		}
		if allDone {
			ready = append(ready, t)
		}
	}
	return ready
}

func (m *Manager) InjectArtifacts(taskID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.tasks[taskID]
	if t == nil {
		return
	}
	for _, depID := range t.DependsOn {
		dep := m.tasks[depID]
		if dep != nil && dep.Result != nil {
			t.Artifacts = append(t.Artifacts, dep.Result.Artifacts...)
		}
	}
}

func (m *Manager) AllDone() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, t := range m.tasks {
		if t.Status != StatusDone && t.Status != StatusFailed {
			return false
		}
	}
	return len(m.tasks) > 0
}

func (m *Manager) AllTasks() []*Task {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Task, 0, len(m.order))
	for _, id := range m.order {
		result = append(result, m.tasks[id])
	}
	return result
}

func (m *Manager) Summary() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var s string
	for _, id := range m.order {
		t := m.tasks[id]
		s += fmt.Sprintf("[%s] %s (%s) - %s\n", t.ID, t.Title, t.AgentName, t.Status)
		if t.Result != nil {
			s += fmt.Sprintf("  输出: %s\n", t.Result.Output)
			for _, a := range t.Result.Artifacts {
				s += fmt.Sprintf("  产出物 [%s]: %s\n", a.Name, truncate(a.Content, 200))
			}
		}
	}
	return s
}

func (m *Manager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks = make(map[string]*Task)
	m.order = nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
