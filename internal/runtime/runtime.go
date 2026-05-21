package runtime

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/chzyer/readline"
	"github.com/horizon6666/max-team/internal/agent"
	"github.com/horizon6666/max-team/internal/audit"
	"github.com/horizon6666/max-team/internal/bus"
	"github.com/horizon6666/max-team/internal/config"
	"github.com/horizon6666/max-team/internal/gate"
	"github.com/horizon6666/max-team/internal/llm"
	"github.com/horizon6666/max-team/internal/scheduler"
	"github.com/horizon6666/max-team/internal/task"
	"github.com/horizon6666/max-team/internal/tool"
)

const (
	colorReset  = "\033[0m"
	colorBlue   = "\033[34m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

type cliState int

const (
	stateIdle cliState = iota
	stateWaitingReply
	stateWaitingApproval
	stateWaitingClarify
)

type Runtime struct {
	cfg       *config.Config
	agentsCfg *config.AgentsConfig
	bus       *bus.MessageBus
	audit     *audit.Logger
	router    *llm.Router
	registry  *tool.Registry
	taskMgr   *task.Manager
	scheduler *scheduler.Scheduler
	agents    []agent.Agent
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	rl        *readline.Instance

	state       cliState
	pendingMsg  bus.Message
	clarifyFrom string
}

func New(cfg *config.Config, agentsCfg *config.AgentsConfig) *Runtime {
	return &Runtime{
		cfg:       cfg,
		agentsCfg: agentsCfg,
	}
}

func (rt *Runtime) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	rt.cancel = cancel

	rt.bus = bus.New()
	rt.audit = audit.New(rt.cfg.Audit)
	rt.router = llm.NewRouter(rt.cfg.LLM, rt.audit)
	rt.taskMgr = task.NewManager()

	rt.registry = tool.NewRegistry()
	tool.RegisterBuiltins(rt.registry, rt.cfg.Project.Root, rt.cfg.Project.Sandbox)

	approvalGate := gate.New(rt.bus)
	rt.scheduler = scheduler.New(rt.taskMgr, rt.bus, approvalGate, rt.audit)

	for _, ac := range rt.agentsCfg.Agents {
		a := rt.createAgent(ac)
		if a == nil {
			continue
		}
		if err := a.Start(ctx); err != nil {
			log.Printf("[runtime] failed to start agent %s: %v", ac.Name, err)
			continue
		}
		rt.agents = append(rt.agents, a)
	}

	for _, a := range rt.agents {
		a := a
		rt.wg.Add(1)
		go func() {
			defer rt.wg.Done()
			a.Run(ctx)
		}()
	}

	rt.wg.Add(1)
	go func() {
		defer rt.wg.Done()
		rt.scheduler.Run(ctx)
	}()

	log.Printf("[runtime] started with %d agents", len(rt.agents))
	return nil
}

func (rt *Runtime) createAgent(ac config.AgentConfig) agent.Agent {
	tools := rt.registry.ForAgent(ac.Tools)

	switch ac.Role {
	case "team_lead":
		maxAgent := agent.NewMax(ac, rt.router, tools, rt.bus, rt.audit, rt.taskMgr)
		maxAgent.SetSubmitter(rt.scheduler)
		return maxAgent
	case "coder":
		return agent.NewLeo(ac, rt.router, tools, rt.bus, rt.audit, rt.cfg.Project.Root)
	default:
		log.Printf("[runtime] unknown role %s for agent %s, skipping", ac.Role, ac.Name)
		return nil
	}
}

func (rt *Runtime) Run() {
	inbox := rt.bus.Subscribe("user")
	inputCh := make(chan string)
	stdinClosed := false

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          "> ",
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		log.Fatalf("readline init failed: %v", err)
	}
	defer rl.Close()
	rt.rl = rl

	go func() {
		for {
			line, err := rl.Readline()
			if err != nil {
				if err == readline.ErrInterrupt || err == io.EOF {
					close(inputCh)
					return
				}
				continue
			}
			inputCh <- line
		}
	}()

	rt.printWelcome()
	rt.state = stateIdle

	for {
		select {
		case input, ok := <-inputCh:
			if !ok {
				stdinClosed = true
				if rt.state == stateIdle {
					return
				}
				inputCh = nil
				continue
			}
			rt.handleInput(strings.TrimSpace(input))

		case msg := <-inbox:
			rt.handleMessage(msg)
			if stdinClosed && rt.state == stateIdle {
				return
			}
		}
	}
}

func (rt *Runtime) printf(format string, a ...any) {
	fmt.Fprintf(rt.rl.Stdout(), format, a...)
}

func (rt *Runtime) handleInput(input string) {
	if input == "" {
		return
	}
	if input == "quit" || input == "exit" {
		rt.printf("\n%s再见！%s\n", colorCyan, colorReset)
		rt.Stop()
		os.Exit(0)
	}

	switch rt.state {
	case stateIdle:
		rt.state = stateWaitingReply
		rt.printf("%s⏳ Max 正在分析你的需求...%s\n", colorYellow, colorReset)
		rt.bus.Send(bus.Message{
			From:    "user",
			To:      "max",
			Type:    bus.MsgUserInput,
			Payload: input,
		})

	case stateWaitingApproval:
		answer := strings.ToLower(input)
		approved := answer == "y" || answer == "yes"
		rt.bus.Send(bus.Message{
			From:    "user",
			To:      rt.pendingMsg.From,
			Type:    bus.MsgApprovalResp,
			Payload: approved,
			ReplyTo: rt.pendingMsg.ID,
		})
		if approved {
			rt.printf("%s✅ 已批准，任务开始执行...%s\n", colorGreen, colorReset)
			rt.state = stateWaitingReply
		} else {
			rt.printf("%s❌ 已拒绝%s\n", colorRed, colorReset)
			rt.state = stateWaitingReply
		}

	case stateWaitingClarify:
		rt.bus.Send(bus.Message{
			From:    "user",
			To:      rt.clarifyFrom,
			Type:    bus.MsgUserInput,
			Payload: input,
		})
		rt.state = stateWaitingReply

	case stateWaitingReply:
		rt.printf("%s⏳ 请等待当前任务完成...%s\n", colorYellow, colorReset)
	}
}

func (rt *Runtime) handleMessage(msg bus.Message) {
	switch msg.Type {
	case bus.MsgUserReply:
		rt.printf("\n%s%s[Max]%s %s\n\n", colorBold, colorBlue, colorReset, msg.Payload)
		rt.state = stateIdle

	case bus.MsgApprovalReq:
		rt.pendingMsg = msg
		rt.state = stateWaitingApproval
		tasks, ok := msg.Payload.([]*task.Task)
		if ok {
			fmt.Fprint(rt.rl.Stdout(), gate.FormatApproval(tasks))
		}

	case bus.MsgProgress:
		rt.printf("%s💬 %s%s\n", colorYellow, msg.Payload, colorReset)

	case bus.MsgNeedClarify:
		rt.clarifyFrom = msg.From
		rt.state = stateWaitingClarify
		rt.printf("\n%s❓ %s%s\n", colorCyan, msg.Payload, colorReset)

	case bus.MsgTaskFailed:
		if result, ok := msg.Payload.(*task.Result); ok {
			rt.printf("%s⚠️  任务失败: %s%s\n", colorRed, result.Error, colorReset)
		}
	}
}

func (rt *Runtime) printWelcome() {
	rt.printf("\n")
	rt.printf("%s%s╔══════════════════════════════════════╗%s\n", colorBold, colorCyan, colorReset)
	rt.printf("%s%s║          Max Team CLI v0.1           ║%s\n", colorBold, colorCyan, colorReset)
	rt.printf("%s%s╚══════════════════════════════════════╝%s\n", colorBold, colorCyan, colorReset)
	rt.printf("\n")
	rt.printf("  输入需求，Max 会拆解并协调团队完成。\n")
	rt.printf("  输入 %squit%s 退出。\n\n", colorYellow, colorReset)
}

func (rt *Runtime) Stop() {
	log.Printf("[runtime] shutting down...")
	if rt.cancel != nil {
		rt.cancel()
	}
	rt.wg.Wait()
	if rt.audit != nil {
		rt.audit.Close()
	}
	log.Printf("[runtime] stopped")
}
