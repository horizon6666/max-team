package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/horizon6666/max-team/internal/audit"
	"github.com/horizon6666/max-team/internal/bus"
	"github.com/horizon6666/max-team/internal/config"
	"github.com/horizon6666/max-team/internal/llm"
	"github.com/horizon6666/max-team/internal/task"
	"github.com/horizon6666/max-team/internal/tool"
)

type PlanSubmitter interface {
	SubmitPlan(tasks []*task.Task) error
}

type MaxAgent struct {
	BaseAgent
	taskMgr   *task.Manager
	submitter PlanSubmitter
}

func NewMax(cfg config.AgentConfig, r *llm.Router, tools []tool.Tool, b *bus.MessageBus, a *audit.Logger, tm *task.Manager) *MaxAgent {
	return &MaxAgent{
		BaseAgent: NewBaseAgent(cfg, r, tools, b, a),
		taskMgr:   tm,
	}
}

func (m *MaxAgent) SetSubmitter(s PlanSubmitter) {
	m.submitter = s
}

func (m *MaxAgent) Run(ctx context.Context) {
	log.Printf("[max] entering message loop")
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-m.inbox:
			switch msg.Type {
			case bus.MsgUserInput:
				m.handleUserInput(ctx, msg)
			case bus.MsgAllTasksDone:
				m.handleAllDone(ctx, msg)
			case bus.MsgTaskFailed:
				m.handleTaskFailed(ctx, msg)
			case bus.MsgNeedClarify:
				m.handleClarify(ctx, msg)
			}
		}
	}
}

func (m *MaxAgent) handleUserInput(ctx context.Context, msg bus.Message) {
	userInput, ok := msg.Payload.(string)
	if !ok {
		return
	}
	log.Printf("[max] received user input: %s", truncateLog(userInput, 100))

	m.setStatus("thinking", "分析用户需求")
	defer m.setStatus("idle", "")

	m.taskMgr.Reset()

	result, err := m.RunLLM(ctx, userInput)
	if err != nil {
		log.Printf("[max] LLM error: %v", err)
		m.audit.Error("max", "LLM call failed", err)
		m.replyToUser(msg, fmt.Sprintf("抱歉，分析需求时出错：%v", err))
		return
	}

	if !strings.Contains(result, "[") {
		m.replyToUser(msg, result)
		return
	}

	tasks, err := m.parseTasks(result)
	if err != nil {
		m.replyToUser(msg, result)
		return
	}

	if len(tasks) == 0 {
		m.replyToUser(msg, result)
		return
	}

	if m.submitter == nil {
		m.replyToUser(msg, "调度器未就绪。")
		return
	}

	planSummary := fmt.Sprintf("📋 任务计划（共 %d 个任务）：\n", len(tasks))
	for i, t := range tasks {
		planSummary += fmt.Sprintf("  %d. [%s] %s\n", i+1, t.AgentName, t.Title)
	}
	log.Printf("[max] %s", planSummary)

	if err := m.submitter.SubmitPlan(tasks); err != nil {
		log.Printf("[max] submit plan failed: %v", err)
		m.replyToUser(msg, fmt.Sprintf("提交计划失败：%v", err))
		return
	}

	m.replyToUser(msg, planSummary+"任务已提交，等待审批...")
}

func (m *MaxAgent) handleAllDone(ctx context.Context, msg bus.Message) {
	log.Printf("[max] all tasks done, generating summary")
	m.setStatus("thinking", "生成完成报告")
	defer m.setStatus("idle", "")

	summary := m.taskMgr.Summary()
	prompt := fmt.Sprintf(`所有任务已完成。以下是任务执行结果：

%s

请生成一份简洁的完成报告，包括：
1. 完成了什么
2. 关键产出物
3. 注意事项（如果有）`, summary)

	m.ResetHistory()
	report, err := m.RunLLM(ctx, prompt)
	if err != nil {
		log.Printf("[max] generate report failed: %v", err)
		m.Send("user", bus.MsgUserReply, "所有任务已完成。\n\n"+summary)
		return
	}

	m.Send("user", bus.MsgUserReply, report)
}

func (m *MaxAgent) handleTaskFailed(_ context.Context, msg bus.Message) {
	result, ok := msg.Payload.(*task.Result)
	if !ok {
		return
	}
	log.Printf("[max] task failed: %s - %s", result.TaskID, result.Error)

	t := m.taskMgr.Get(result.TaskID)
	if t == nil {
		return
	}

	m.Send("user", bus.MsgUserReply, fmt.Sprintf("任务失败：%s\n错误：%s\n已重试 %d 次", t.Title, result.Error, t.RetryCount))
}

func (m *MaxAgent) handleClarify(_ context.Context, msg bus.Message) {
	question, _ := msg.Payload.(string)
	m.Send("user", bus.MsgNeedClarify, fmt.Sprintf("[%s 提问] %s", msg.From, question))
}

func (m *MaxAgent) replyToUser(original bus.Message, content string) {
	m.bus.Send(bus.Message{
		From:    "max",
		To:      "user",
		Type:    bus.MsgUserReply,
		Payload: content,
		ReplyTo: original.ID,
	})
}

func (m *MaxAgent) parseTasks(raw string) ([]*task.Task, error) {
	cleaned := extractJSON(raw)

	var items []struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		AgentName   string `json:"agent_name"`
		DependsOn   []int  `json:"depends_on"`
	}
	if err := json.Unmarshal([]byte(cleaned), &items); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}

	tasks := make([]*task.Task, len(items))
	for i, item := range items {
		t := &task.Task{
			Title:       item.Title,
			Description: item.Description,
			AgentName:   item.AgentName,
		}
		if t.AgentName == "" {
			t.AgentName = "leo"
		}
		tasks[i] = t
	}

	for i, item := range items {
		for _, depIdx := range item.DependsOn {
			if depIdx >= 0 && depIdx < len(tasks) && depIdx != i {
				tasks[i].DependsOn = append(tasks[i].DependsOn, tasks[depIdx].ID)
			}
		}
	}

	return tasks, nil
}

func extractJSON(s string) string {
	start := -1
	for i, c := range s {
		if c == '[' {
			start = i
			break
		}
	}
	if start == -1 {
		return s
	}

	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return s[start:]
}

func truncateLog(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
