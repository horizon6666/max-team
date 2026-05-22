package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/horizon6666/max-team/internal/config"
	"github.com/horizon6666/max-team/internal/runtime"
)

func main() {
	configPath := flag.String("config", "config/config.yaml", "配置文件路径")
	agentsPath := flag.String("agents", "config/agents.yaml", "Agent 配置文件路径")
	model := flag.String("model", "", "覆盖所有 Agent 的模型名称")
	provider := flag.String("provider", "", "覆盖 Provider (anthropic/openai)")
	baseURL := flag.String("base-url", "", "覆盖 LLM API 地址")
	apiKey := flag.String("api-key", "", "覆盖 API Key")
	mode := flag.String("mode", "", "运行模式 (cli/web)")
	port := flag.Int("port", 0, "覆盖 Web 服务端口")
	flag.Parse()

	logDir := "logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "创建日志目录失败: %v\n", err)
		os.Exit(1)
	}
	logFile, err := os.OpenFile(filepath.Join(logDir, "max-team.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开日志文件失败: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	cfg, agentsCfg, err := config.Load(*configPath, *agentsPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	if *model != "" {
		cfg.LLM.DefaultModel = *model
		for i := range agentsCfg.Agents {
			agentsCfg.Agents[i].Model = *model
		}
	}
	if *provider != "" {
		cfg.LLM.DefaultProvider = *provider
		for i := range agentsCfg.Agents {
			agentsCfg.Agents[i].Provider = *provider
		}
	}
	if *baseURL != "" {
		cfg.LLM.BaseURL = *baseURL
		for _, pcfg := range cfg.LLM.Providers {
			pcfg.BaseURL = *baseURL
			cfg.LLM.Providers[cfg.LLM.DefaultProvider] = pcfg
			break
		}
	}
	if *apiKey != "" {
		cfg.LLM.APIKey = *apiKey
		for _, pcfg := range cfg.LLM.Providers {
			pcfg.APIKey = *apiKey
			cfg.LLM.Providers[cfg.LLM.DefaultProvider] = pcfg
			break
		}
	}
	if *mode != "" {
		cfg.Server.Mode = *mode
	}
	if *port != 0 {
		cfg.Server.Port = *port
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt := runtime.New(cfg, agentsCfg)
	if err := rt.Start(ctx); err != nil {
		log.Fatalf("启动失败: %v", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("收到退出信号")
		cancel()
		rt.Stop()
		os.Exit(0)
	}()

	rt.Run()
	rt.Stop()
}
