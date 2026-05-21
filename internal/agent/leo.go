package agent

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"

	"github.com/horizon6666/max-team/internal/audit"
	"github.com/horizon6666/max-team/internal/bus"
	"github.com/horizon6666/max-team/internal/config"
	"github.com/horizon6666/max-team/internal/llm"
	"github.com/horizon6666/max-team/internal/task"
	"github.com/horizon6666/max-team/internal/tool"
)

type LeoAgent struct {
	BaseAgent
	projectRoot string
}

func NewLeo(cfg config.AgentConfig, r *llm.Router, tools []tool.Tool, b *bus.MessageBus, a *audit.Logger, projectRoot string) *LeoAgent {
	return &LeoAgent{
		BaseAgent:   NewBaseAgent(cfg, r, tools, b, a),
		projectRoot: projectRoot,
	}
}

func (l *LeoAgent) Run(ctx context.Context) {
	log.Printf("[leo] entering message loop")
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-l.inbox:
			if msg.Type == bus.MsgTaskAssign {
				l.handleTask(ctx, msg)
			}
		}
	}
}

func (l *LeoAgent) handleTask(ctx context.Context, msg bus.Message) {
	t, ok := msg.Payload.(*task.Task)
	if !ok {
		return
	}
	log.Printf("[leo] received task: %s - %s", t.ID, t.Title)

	l.ResetHistory()

	prompt := l.buildPrompt(t)
	result, err := l.RunLLM(ctx, prompt)
	if err != nil {
		log.Printf("[leo] task %s failed: %v", t.ID, err)
		l.audit.Error("leo", fmt.Sprintf("task %s failed", t.ID), err)
		l.Send("scheduler", bus.MsgTaskFailed, &task.Result{
			TaskID:  t.ID,
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	diff := l.collectDiff()

	artifacts := []task.Artifact{}
	if diff != "" {
		artifacts = append(artifacts, task.Artifact{
			Name:    "git_diff",
			Type:    "diff",
			Content: diff,
		})
	}
	if result != "" {
		artifacts = append(artifacts, task.Artifact{
			Name:    "output",
			Type:    "text",
			Content: result,
		})
	}

	log.Printf("[leo] task %s completed, artifacts: %d", t.ID, len(artifacts))

	l.Send("scheduler", bus.MsgTaskResult, &task.Result{
		TaskID:    t.ID,
		Success:   true,
		Output:    result,
		Artifacts: artifacts,
	})
}

func (l *LeoAgent) buildPrompt(t *task.Task) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## 任务：%s\n\n", t.Title))
	sb.WriteString(t.Description)
	sb.WriteString("\n")

	if len(t.Artifacts) > 0 {
		sb.WriteString("\n## 上游任务产出物\n\n")
		for _, a := range t.Artifacts {
			sb.WriteString(fmt.Sprintf("### %s (%s)\n```\n%s\n```\n\n", a.Name, a.Type, a.Content))
		}
	}

	sb.WriteString("\n请完成以上任务。使用提供的工具来读写文件、执行命令。完成后简要说明你做了什么。")
	return sb.String()
}

func (l *LeoAgent) collectDiff() string {
	cmd := exec.Command("git", "diff")
	cmd.Dir = l.projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return string(output)
}
