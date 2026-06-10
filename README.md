# Logos — AI Agent 状态与知识管理系统

Logos 是一个 AI agent 的状态与知识管理工具。通过标准化的 `.iroll`（智能卷轴）包格式，存储 agent 的人格、记忆、运行循环、知识和资源。

**核心原则：不集成任何 agent 能力，让 agent 用我们。**

## 快速开始

```bash
# 构建
cd iroll
go build -o ../logos .
cd ..

# 查看状态
./logos status

# 从 Layerfile 构建一个 agent
./logos roll build -f examples/base-agent/Layerfile -t my-agent

# 创建页面，开始对话
./logos page new my-agent --cwd .

# 读取 agent 的上下文
./logos page get-context --cwd .
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

### Loop（运行循环）

Loop 决定 agent 可以自主选择去做什么，但 Logos 不负责执行、调度或规划工作。

- `loop` 保存可重复使用的行为种子，没有全局执行状态。
- `loop_runs` 保存 page 独立的运行状态与生命记录。
- 每个 page 最多一个 active 主 run，可有多个一层子 run。
- run 生命周期只有 `active → completed | aborted`。
- `page get-context` 动态注入当前 page 的 `loop.focus` 和全局 `loop.available`；原始 `pages.context` 不保存运行状态。
- run 结束后事实不可修改，但可以追加或替换 reflection。

### Book（知识书籍）

Book Bundle 位于 `Resources/books/<book-id>/`，由 `manifest.json` 和三个 Parquet 文件组成。构建时自动校验并注册；查询时由调用方提供标签，Logos 执行确定性的精确标签检索与评分。

Logos 不负责从问题中提取标签，也不负责生成答案。

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
logos page get-context [name] [--page <id>] [--cwd .]      # 获取上下文（已解析）
logos page update-context [name] --content <json> [--page <id>] [--cwd .]
logos page add-memory [name] --content <text> [--importance 0.5] [--cwd .]
logos page query-dna <name> [--type <type>] [--cwd .]      # 查询 dna（模糊匹配）
```

### 知识书籍

```bash
logos book list [name] [--cwd .]                 # 列出已注册书籍
logos book inspect <book-id> [name] [--cwd .]   # 查看书籍元数据
logos book query --book <id> --tag <tag> [--cwd .]  # 按精确标签检索原文片段
```

### Loop 管理

```bash
logos loop list [--archived] [--cwd .]
logos loop inspect|add|edit|remove|archive|restore ...
logos loop run <name> [--parent <main-run-id>] [--plan <json-or-text>] [--cwd .]
logos loop update [run-id] [--plan <value>] [--progress <value>] [--cwd .]
logos loop complete [run-id] --result <value> [--cwd .]
logos loop abort [run-id] --reason <text> [--result <value>] [--cwd .]
logos loop reflect <run-id> --content <value> [--cwd .]
logos loop current|history|show ...
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
| loop | 可复用行为种子（name, content, weight, archived_at） |
| loop_runs | page 独立的运行状态与不可变生命记录 |
| pages | 页面上下文（page_id, context） |
| memory | 记忆存储（content, importance） |
| book | Book Bundle 元数据和资源路径 |
| history | 构建历史 |

### system.db（全局）

| 表 | 说明 |
|----|------|
| page_index | 页面索引 |
| active_page | 每个 cwd 的活跃页面 |
| config | 全局配置 |

## 项目结构

```
ai-logos/
├── logos                     # 编译产物
├── iroll/                    # Go 源码
│   ├── book/                 # Book Bundle 校验与检索
│   ├── builder/              # Layerfile 分层构建
│   ├── cmd/                  # Cobra 命令实现
│   ├── db/                   # SQLite 数据操作
│   ├── safepath/             # 路径安全校验
│   ├── store/                # 存储管理
│   └── go.mod
├── examples/                 # 示例
│   ├── base-agent/           # 基础 agent 模板
│   │   └── books/            # 示例 Book Bundle
│   └── layer2/               # 分层构建示例
├── skills/                   # Agent 使用技能
│   └── logos-1/skill.md
└── docs/                     # 文档
    └── rebot-roll.md
```

当前使用说明以本 README 和 `docs/rebot-roll.md` 为准。`docs/superpowers/` 保存带日期的设计与实施记录，其中可能包含已经被后续设计替代的历史术语和计划状态。

## 技术栈

- Go 1.24, Cobra CLI
- SQLite（go-sqlite3, CGO）
- Parquet（parquet-go）

## 构建

```bash
cd iroll
go build -o ../logos .
```

需要 CGO 环境（go-sqlite3 依赖）。Windows 上需安装 GCC（如 MinGW-w64 或 TDM-GCC）。

## License

MIT
