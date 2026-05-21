package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server  ServerConfig  `yaml:"server"`
	LLM     LLMConfig     `yaml:"llm"`
	Audit   AuditConfig   `yaml:"audit"`
	Project ProjectConfig `yaml:"project"`
}

type ServerConfig struct {
	Port int    `yaml:"port"`
	Mode string `yaml:"mode"`
}

type LLMConfig struct {
	Provider     string        `yaml:"provider"`
	APIKey       string        `yaml:"api_key"`
	BaseURL      string        `yaml:"base_url"`
	DefaultModel string        `yaml:"default_model"`
	MaxRetries   int           `yaml:"max_retries"`
	Timeout      time.Duration `yaml:"timeout"`
}

type AuditConfig struct {
	Enabled bool   `yaml:"enabled"`
	Output  string `yaml:"output"`
	Level   string `yaml:"level"`
}

type ProjectConfig struct {
	Root    string `yaml:"root"`
	Sandbox bool   `yaml:"sandbox"`
}

type AgentsConfig struct {
	Agents []AgentConfig `yaml:"agents"`
}

type AgentConfig struct {
	Name         string            `yaml:"name"`
	Role         string            `yaml:"role"`
	Model        string            `yaml:"model"`
	SystemPrompt string            `yaml:"system_prompt"`
	Tools        []string          `yaml:"tools"`
	MCPServers   []MCPServerConfig `yaml:"mcp_servers"`
	Constraints  Constraints       `yaml:"constraints"`
}

type Constraints struct {
	AllowedDirs     []string      `yaml:"allowed_dirs"`
	BlockedPatterns []string      `yaml:"blocked_patterns"`
	WritePatterns   []string      `yaml:"write_patterns"`
	AllowedCommands []string      `yaml:"allowed_commands"`
	MaxTokens       int           `yaml:"max_tokens"`
	Timeout         time.Duration `yaml:"timeout"`
}

type MCPServerConfig struct {
	Name    string            `yaml:"name"`
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args"`
	Env     map[string]string `yaml:"env,omitempty"`
}

func Load(configPath, agentsPath string) (*Config, *AgentsConfig, error) {
	cfg := &Config{}
	if err := loadYAML(configPath, cfg); err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}
	expandEnv(&cfg.LLM.APIKey)
	expandEnv(&cfg.LLM.BaseURL)
	setConfigDefaults(cfg)

	agents := &AgentsConfig{}
	if err := loadYAML(agentsPath, agents); err != nil {
		return nil, nil, fmt.Errorf("load agents: %w", err)
	}
	for i := range agents.Agents {
		setAgentDefaults(&agents.Agents[i])
	}

	return cfg, agents, nil
}

func loadYAML(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, out)
}

func expandEnv(s *string) {
	if strings.HasPrefix(*s, "${") && strings.HasSuffix(*s, "}") {
		envKey := (*s)[2 : len(*s)-1]
		*s = os.Getenv(envKey)
	}
}

func setConfigDefaults(cfg *Config) {
	if cfg.Server.Mode == "" {
		cfg.Server.Mode = "cli"
	}
	if cfg.LLM.MaxRetries == 0 {
		cfg.LLM.MaxRetries = 3
	}
	if cfg.LLM.Timeout == 0 {
		cfg.LLM.Timeout = 120 * time.Second
	}
	if cfg.Audit.Output == "" {
		cfg.Audit.Output = "logs/audit.jsonl"
	}
	if cfg.Audit.Level == "" {
		cfg.Audit.Level = "info"
	}
	if cfg.Project.Root == "" {
		cfg.Project.Root = "."
	}
}

func setAgentDefaults(ac *AgentConfig) {
	if ac.Constraints.MaxTokens == 0 {
		ac.Constraints.MaxTokens = 4096
	}
	if ac.Constraints.Timeout == 0 {
		ac.Constraints.Timeout = 120 * time.Second
	}
}
