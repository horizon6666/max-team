package gate

import (
	"fmt"
	"strings"
	"time"

	"github.com/horizon6666/max-team/internal/bus"
	"github.com/horizon6666/max-team/internal/task"
)

type ApprovalGate struct {
	bus *bus.MessageBus
}

func New(b *bus.MessageBus) *ApprovalGate {
	return &ApprovalGate{bus: b}
}

func (g *ApprovalGate) RequestApproval(tasks []*task.Task) (bool, error) {
	reply, err := g.bus.SendAndWait(bus.Message{
		From:    "scheduler",
		To:      "user",
		Type:    bus.MsgApprovalReq,
		Payload: tasks,
	}, 5*time.Minute)
	if err != nil {
		return false, fmt.Errorf("approval timeout: %w", err)
	}

	approved, ok := reply.Payload.(bool)
	if !ok {
		return false, fmt.Errorf("invalid approval response")
	}
	return approved, nil
}

func FormatApproval(tasks []*task.Task) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n\033[32m========== 任务审批（共 %d 项）==========\033[0m\n", len(tasks)))
	for i, t := range tasks {
		deps := "无"
		if len(t.DependsOn) > 0 {
			deps = strings.Join(t.DependsOn, ", ")
		}
		sb.WriteString(fmt.Sprintf("  \033[32m%d.\033[0m [%s] %s\n", i+1, t.AgentName, t.Title))
		sb.WriteString(fmt.Sprintf("     %s\n", t.Description))
		sb.WriteString(fmt.Sprintf("     依赖: %s\n", deps))
	}
	sb.WriteString("\033[32m==========================================\033[0m\n")
	sb.WriteString("\033[32m是否批准执行？(y/n): \033[0m")
	return sb.String()
}
