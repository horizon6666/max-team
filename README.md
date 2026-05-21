# Max Team

个人工作团队多 Agent 协作系统 — Go + Claude API

## 团队成员

| 名字 | 角色 | 职责 | 模型 |
|------|------|------|------|
| **Max** | Team Lead | 需求理解、任务拆解、方案确认、进度追踪 | Claude Sonnet |
| **Leo** | Coder | 写代码、改 Bug、重构、Git 操作 | Claude Opus |
| **Ada** | Architect | 方案设计、架构评审、文档输出 | Claude Opus |
| **Ray** | Tester | 编写测试、执行测试、覆盖率分析 | Claude Sonnet |
| **Sam** | Ops | 日志查询、监控排查、问题定位 | Claude Haiku |

## 快速开始

### 环境要求

- Go 1.23+
- Claude API Key（直连 Anthropic 或公司 LLM Proxy）

### 构建

```bash
go build -o max-team ./cmd/max-team/
```

### 配置

编辑 `config/config.yaml` 设置 LLM 连接：

```yaml
llm:
  api_key: ${ANTHROPIC_AUTH_TOKEN}    # 环境变量名
  base_url: ${ANTHROPIC_BASE_URL}     # API 地址
  default_model: auto-max             # 模型名称
```

### 运行

```bash
# 设置环境变量
export ANTHROPIC_AUTH_TOKEN="你的 API Key"
export ANTHROPIC_BASE_URL="https://api.anthropic.com"  # 或公司 LLM Proxy 地址

# 启动
./max-team
```

### CLI 参数

命令行参数优先级高于配置文件：

```bash
# 指定模型
./max-team --model claude-opus-4

# 使用 OpenAI 兼容模型（GPT、GLM、DeepSeek、MiniMax 等）
./max-team --provider openai --model gpt-4o --base-url https://api.openai.com --api-key sk-xxx

# 使用 DeepSeek
./max-team --provider openai --model deepseek-chat --base-url https://api.deepseek.com --api-key sk-xxx

# 组合使用
./max-team --model auto-max --base-url http://llm-proxy.example.com --api-key sk-xxx

# 指定配置文件
./max-team --config path/to/config.yaml --agents path/to/agents.yaml
```

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--model` | 覆盖所有 Agent 的模型名称 | 配置文件中的值 |
| `--provider` | 覆盖 Provider (`anthropic`/`openai`) | 配置文件中的值 |
| `--base-url` | 覆盖 LLM API 地址 | 配置文件中的值 |
| `--api-key` | 覆盖 API Key | 配置文件中的值 |
| `--config` | 全局配置文件路径 | `config/config.yaml` |
| `--agents` | Agent 配置文件路径 | `config/agents.yaml` |

### 支持的模型

| Provider | 模型 | 说明 |
|----------|------|------|
| `anthropic` | Claude (auto-max, claude-opus-4 等) | 默认，Anthropic 协议 |
| `openai` | GPT-4o, GPT-4, GPT-3.5 等 | OpenAI 官方 |
| `openai` | DeepSeek (deepseek-chat 等) | OpenAI 兼容协议 |
| `openai` | GLM (glm-4 等) | 智谱，OpenAI 兼容 |
| `openai` | MiniMax, Moonshot, 通义千问 等 | OpenAI 兼容 |

### 使用示例

```
╔══════════════════════════════════════╗
║          Max Team CLI v0.1           ║
╚══════════════════════════════════════╝

> 给项目写一个 hello world 的 main.go
⏳ Max 正在分析你的需求...

========== 任务审批（共 1 项）==========
  1. [leo] 创建 main.go 文件
     在项目根目录下创建 hello world 程序
     依赖: 无
==========================================
是否批准执行？(y/n): y
✅ 已批准，任务开始执行...

[Max] 任务已完成！Leo 在项目根目录创建了 main.go 文件...

> quit
```

**交互流程：** 输入需求 → Max 拆解任务 → 审批(y/n) → Leo 执行 → Max 汇总结果

## 项目结构

```
max-team/
├── cmd/max-team/main.go          # 入口
├── config/
│   ├── config.yaml               # 全局配置
│   └── agents.yaml               # Agent 配置
└── internal/
    ├── agent/                    # Agent 系统 (Max + Leo)
    ├── audit/                    # 审计日志 (JSONL)
    ├── bus/                      # 消息总线
    ├── config/                   # 配置加载
    ├── gate/                     # 审批门控
    ├── llm/                      # LLM 路由 (Claude API)
    ├── mcp/                      # MCP 客户端 (Phase 2)
    ├── runtime/                  # 运行时 + CLI
    ├── scheduler/                # DAG 调度器
    ├── task/                     # 任务管理
    └── tool/                     # 工具系统 (8 个内置工具)
```

## 文档

- [概要设计](https://horizon6666.github.io/max-team/overview-design.html)
- [技术设计](https://horizon6666.github.io/max-team/technical-design.html)
- [详细设计](https://horizon6666.github.io/max-team/detailed-design.html)
