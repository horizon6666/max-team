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
