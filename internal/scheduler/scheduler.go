package scheduler

import (
	"context"
	"fmt"
	"log"

	"github.com/horizon6666/max-team/internal/audit"
	"github.com/horizon6666/max-team/internal/bus"
	"github.com/horizon6666/max-team/internal/gate"
	"github.com/horizon6666/max-team/internal/task"
)

type Scheduler struct {
	taskMgr *task.Manager
	bus     *bus.MessageBus
	gate    *gate.ApprovalGate
	audit   *audit.Logger
	inbox   <-chan bus.Message
}

func New(tm *task.Manager, b *bus.MessageBus, g *gate.ApprovalGate, a *audit.Logger) *Scheduler {
	return &Scheduler{
		taskMgr: tm,
		bus:     b,
		gate:    g,
		audit:   a,
		inbox:   b.Subscribe("scheduler"),
	}
}

func (s *Scheduler) SubmitPlan(tasks []*task.Task) error {
	for _, t := range tasks {
		if err := s.taskMgr.Create(t); err != nil {
			return fmt.Errorf("create task: %w", err)
		}
	}

	taskIDs := make([]string, len(tasks))
	deps := make(map[string][]string)
	for i, t := range tasks {
		taskIDs[i] = t.ID
		deps[t.ID] = t.DependsOn
	}
	if err := validateDAG(taskIDs, deps); err != nil {
		return fmt.Errorf("invalid plan: %w", err)
	}

	for _, t := range tasks {
		s.taskMgr.Transition(t.ID, task.StatusPlanned)
	}

	approved, err := s.gate.RequestApproval(tasks)
	if err != nil {
		return fmt.Errorf("approval: %w", err)
	}
	if !approved {
		log.Printf("[scheduler] plan rejected by user")
		s.bus.Send(bus.Message{
			From:    "scheduler",
			To:      "user",
			Type:    bus.MsgUserReply,
			Payload: "计划已被拒绝。",
		})
		return fmt.Errorf("plan rejected")
	}

	s.audit.Info(audit.EventApproval, "scheduler", map[string]any{
		"action": "approved",
		"tasks":  len(tasks),
	})

	for _, t := range tasks {
		s.taskMgr.Transition(t.ID, task.StatusApproved)
	}

	s.dispatchReady()
	return nil
}

func (s *Scheduler) Run(ctx context.Context) {
	log.Printf("[scheduler] entering message loop")
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-s.inbox:
			switch msg.Type {
			case bus.MsgTaskResult:
				s.handleTaskResult(msg)
			case bus.MsgTaskFailed:
				s.handleTaskFailed(msg)
			}
		}
	}
}

func (s *Scheduler) dispatchReady() {
	ready := s.taskMgr.ReadyToDispatch()
	for _, t := range ready {
		s.taskMgr.InjectArtifacts(t.ID)
		s.taskMgr.Transition(t.ID, task.StatusDispatched)
		s.taskMgr.Transition(t.ID, task.StatusRunning)

		log.Printf("[scheduler] dispatching task %s (%s) to %s", t.ID, t.Title, t.AgentName)
		s.audit.Info(audit.EventTaskUpdate, "scheduler", map[string]any{
			"action": "dispatch",
			"task":   t.ID,
			"agent":  t.AgentName,
		})

		s.bus.Send(bus.Message{
			From:    "scheduler",
			To:      t.AgentName,
			Type:    bus.MsgTaskAssign,
			Payload: t,
		})
	}
}

func (s *Scheduler) handleTaskResult(msg bus.Message) {
	result, ok := msg.Payload.(*task.Result)
	if !ok {
		return
	}

	log.Printf("[scheduler] task %s completed", result.TaskID)
	s.taskMgr.SetResult(result.TaskID, result)
	s.taskMgr.Transition(result.TaskID, task.StatusDone)

	s.audit.Info(audit.EventTaskUpdate, "scheduler", map[string]any{
		"action": "done",
		"task":   result.TaskID,
	})

	if s.taskMgr.AllDone() {
		log.Printf("[scheduler] all tasks done")
		s.bus.Send(bus.Message{
			From:    "scheduler",
			To:      "max",
			Type:    bus.MsgAllTasksDone,
			Payload: s.taskMgr.Summary(),
		})
		return
	}

	s.dispatchReady()
}

func (s *Scheduler) handleTaskFailed(msg bus.Message) {
	result, ok := msg.Payload.(*task.Result)
	if !ok {
		return
	}

	t := s.taskMgr.Get(result.TaskID)
	if t == nil {
		return
	}

	log.Printf("[scheduler] task %s failed: %s (retry %d/%d)", result.TaskID, result.Error, t.RetryCount, t.MaxRetries)

	s.taskMgr.SetResult(result.TaskID, result)

	if t.RetryCount < t.MaxRetries {
		t.RetryCount++
		s.taskMgr.Transition(result.TaskID, task.StatusFailed)
		s.taskMgr.Transition(result.TaskID, task.StatusDispatched)
		s.taskMgr.Transition(result.TaskID, task.StatusRunning)

		log.Printf("[scheduler] retrying task %s (attempt %d)", result.TaskID, t.RetryCount)
		s.bus.Send(bus.Message{
			From:    "scheduler",
			To:      t.AgentName,
			Type:    bus.MsgTaskAssign,
			Payload: t,
		})
		return
	}

	s.taskMgr.Transition(result.TaskID, task.StatusFailed)

	s.bus.Send(bus.Message{
		From:    "scheduler",
		To:      "max",
		Type:    bus.MsgTaskFailed,
		Payload: result,
	})
}
