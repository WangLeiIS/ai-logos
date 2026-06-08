# Logos — AI Agent 记忆管理系统

Logos 是一个 AI agent 的记忆管理工具。通过标准化的 `.iroll`（智能卷轴）包格式，存储 agent 的完整状态：人格、记忆、待办任务和资源。

**核心原则：不集成任何 agent 能力，让 agent 用我们。**

## 快速开始

```bash
# 构建
cd iroll && go build -o ../logos .

# 查看状态
logos status

# 从 Layerfile 构建一个 agent
logos roll build -f examples/base-agent/Layerfile -t my-agent

# 创建页面，开始对话
logos page new my-agent --cwd .

# 读取 agent 的上下文
logos page get-context --cwd .
```

## 核心概念

### iroll 包

`.iroll` 是一个 ZIP 归档，包含 SQLite 数据库（`ai_roll.db`）和资源目录（`Resources/`）。加载后解压到 `~/.iroll/<name>/`。

### Page（页面）

每次对话创建一个 page，继承模板页（page_id=0）的 context。每个工作目录跟踪自己的活跃页面。

### Context（上下文）

JSON 格式的行为指令，支持三种值类型：

| 类型 | 格式 | 示例 |
|------|------|------|
| 纯字符串 | `"key": "value"` | `"system_prompt": "你是一个AI助手"` |
| 文件引用 | `"key": {"@file": "path"}` | 读取 iroll 包内的文件内容 |
| SQL 查询 | `"key": {"@sql": "SELECT ..."}` | 查询 ai_roll.db |

`@file` 和 `@sql` 在读取时解析为实际值，写入时存储原始标记。

### DNA（决策基因）

通过 Q&A 对定义 agent 的底层决策机制。四个维度：

| 维度 | 含义 |
|------|------|
| 认知观 | 如何看待信息和真相 |
| 伦理观 | 如何判断对错 |
| 审美观 | 什么算好的解决方案 |
| 本体观 | 什么在 agent 的世界里是重要的 |

Context 加载时只读取问题（节省 token），答案通过 `query-dna` 按需查询。

### Heartbeat（心跳/待办）

任务表，支持两种类型：

- **once** — 一次性任务，`pending → done`
- **periodic** — 周期任务，始终 `active`，带执行计数器

## CLI 命令

### 系统状态

```bash
logos status                          # 查看状态
```

### 包管理

```bash
logos roll build -f <file> -t <name>  # 从 Layerfile 构建
logos roll load <file>                # 加载 .iroll 文件
logos roll list                       # 列出所有 iroll
logos roll rm <name>                  # 删除 iroll
logos roll save <name> [-o path]      # 导出为 .iroll
logos roll inspect <name>             # 查看详情
logos roll history <name>             # 构建历史
```

### 页面管理

```bash
logos page new <name> [--cwd .]       # 创建页面
logos page current [--cwd .]          # 当前活跃页面
logos page list [name] [--cwd .] [-a] # 列出页面
logos page switch <page-id>           # 切换页面
logos page delete <page-id>           # 删除页面
logos page get-context [--cwd .]      # 获取上下文（已解析）
logos page update-context --content <json> [--cwd .]  # 更新上下文
logos page add-memory --content <text> [--cwd .]      # 添加记忆
logos page query-dna <name> [--cwd .] # 查询 dna（模糊匹配）
```

### 分层构建

Layerfile 支持三条指令：

```dockerfile
FROM base-agent          # 继承基础层
MIGRATE add_field.sql    # 执行 SQL 迁移
COPY logo.png img/       # 复制资源文件
```

## 数据库结构

### ai_roll.db（每个 iroll 包）

| 表 | 说明 |
|----|------|
| metadata | key-value 元数据 |
| dna | 决策基因（name, type, question, answer, weight） |
| heartbeat | 待办任务（type, content, status, result） |
| pages | 页面上下文（page_id, context） |
| memory | 记忆存储（content, importance） |
| history | 构建历史 |

### system.db（全局）

| 表 | 说明 |
|----|------|
| page_index | 页面索引 |
| active_page | 每个 cwd 的活跃页面 |
| config | 全局配置 |

## 项目结构

```
ai-roll-mini/
├── logos.exe                 # 编译产物
├── iroll/                    # Go 源码
│   ├── cmd/                  # Cobra 命令实现
│   │   ├── root.go
│   │   ├── roll.go
│   │   ├── build.go
│   │   ├── page.go
│   │   ├── query_dna.go
│   │   └── ...
│   ├── db/                   # 数据库操作
│   │   └── db.go
│   ├── store/                # 存储管理
│   │   └── system.go
│   └── go.mod
├── examples/                 # 示例
│   ├── base-agent/           # 基础 agent 模板
│   │   ├── Layerfile
│   │   ├── init_schema.sql
│   │   ├── init_data.sql
│   │   └── greeting.txt
│   └── layer2/               # 分层构建示例
├── skills/                   # Agent 使用技能
│   └── logos-1/skill.md
└── docs/                     # 文档
    └── rebot-roll.md
```

## 技术栈

- Go 1.24, Cobra CLI
- SQLite（go-sqlite3, CGO）
- 纯标准库

## 构建

```bash
cd iroll
go build -o ../logos .
```

需要 CGO 环境（go-sqlite3 依赖）。Windows 上需安装 GCC（如 MinGW-w64 或 TDM-GCC）。

## License

MIT
