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

# 使用 Irollfile 构建 agent（默认当前目录 ./Irollfile）
./logos roll build -t my-agent

# 创建页面，开始对话
./logos page new my-agent --cwd .

# 读取 agent 的上下文
./logos page get --cwd .
```

## 核心概念

### iroll 包

`.iroll` 是一个 ZIP 归档，包含两个 SQLite 数据库（`roll-inner.db` + `roll-outer.db`）、资源目录（`Resources/`）和分层描述文件（`layer.json`）。加载后解压到 `~/.iroll/<name>/`。

- `roll-inner.db`（只读蓝图，构建时写入）：metadata / dna / loop（种子）/ skill / book / history，以及模板行（pages/memory/loop_runs 中 page_id='0'）。
- `roll-outer.db`（模板）：pages / memory / loop_runs 的 schema。运行时按 cwd 复制为 `<cwd>/.iroll/<name>.db` 或 `~/.iroll/<name>/<version>/workspace/.<name>.outer.db`。
- 打开方式：outer 为主库，`ATTACH roll-inner.db AS inner`。裸表名指向 outer，访问 inner 表需加 `inner.` 前缀。

### Page（页面）

每次对话创建一个 page，继承模板页（page_id=0）的 context。每个工作目录跟踪自己的活跃页面。

### Context（上下文）

JSON 格式的行为指令，支持三种值类型：

| 类型 | 格式 | 示例 |
|------|------|------|
| 纯字符串 | `"key": "value"` | `"system_prompt": "你是一个AI助手"` |
| 文件引用 | `"key": {"@file": "path"}` | 读取 iroll 包内的文件内容 |
| SQL 查询 | `"key": {"@sql": "SELECT ..."}` | 查询 outer（裸表名）或 inner（带 `inner.` 前缀） |

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
- `page get` 动态注入**顶层** `loop_focus`（当前 page）与 `loop_available`（全局可用种子），以扁平键的形式出现；原始 `pages.context` 不保存运行状态。
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
logos roll build -f <file> -t <name>  # 从 Irollfile 构建
logos roll load <file>                # 加载 .iroll 文件
logos roll list                       # 列出所有 iroll
logos roll rm <name>                  # 删除 iroll
logos roll save <name> [-o path]      # 导出为 .iroll
logos roll inspect <name>             # 查看详情
logos roll history <name>             # 构建历史
```

### irollhub 集成

```bash
logos roll login --hub <url>          # 登录 irollhub，保存 API Key
logos roll logout                     # 清除本地 API Key
logos roll search <keyword>           # 搜索远端包
logos roll push <file.iroll|name> <org>/<pkg>:<ver>  # 发布包版本
logos roll pull <org>/<pkg>[:<ver>]   # 下载并加载包，默认版本 latest
```

### 页面管理

```bash
logos page new <name> [--cwd .]       # 创建页面
logos page list [name] [--cwd .] [-a] # 列出页面
logos page switch <page-id>           # 切换页面
logos page delete <page-id>           # 删除页面
logos page default set <page-id>      # 设置某 cwd 的默认 page
logos page default show               # 查看当前 cwd 的默认 page
logos page default clear              # 清除当前 cwd 的默认 page
logos page get [path] [--page <id>] [--cwd .]                # 获取上下文（全量或单键，已解析）
logos page set <path> <value> [--page <id>] [--cwd .]        # 设置一个 context 键（json-or-text）
logos page set --content '<json>' [--page <id>] [--cwd .]    # 整体替换 context
logos page unset <path> [--page <id>] [--cwd .]              # 删除一个 context 键
logos page alias <name> [--page <id>]                        # 设置别名
logos page alias --clear [--page <id>]                       # 清除别名
logos page query [sql] [--sql <stmt>] [--cwd .]              # 对当前 page 的 outer 库跑 SQL
logos page query-memory [name] [--keyword <text>] [--full] [--cwd .]
logos page query-dna <name> [--type <type>] [--cwd .]      # 查询 dna（模糊匹配）
```

### 知识书籍

```bash
logos book list [name] [--cwd .]                 # 列出已注册书籍
logos book inspect <book-id> [name] [--cwd .]   # 查看书籍元数据
logos book query --book <id> --tag <tag> [--cwd .]  # 按精确标签检索原文片段
```

### Skill 管理

```bash
logos skill list                   # 列出 skill
logos skill show <name>            # 查看 skill 详情
```

### Loop 管理

```bash
logos loop list [--archived] [--stats] [--cwd .]
logos loop inspect|edit|remove|archive|restore ...
logos loop add <name> --type {auto|normal} ...           # 注册种子，--type 决定自动/手动
logos loop run <name> [--parent <main-run-id>] [--plan <json-or-text>] [--cwd .]
logos loop update [run-id] [--plan <value>] [--progress <value>] [--cwd .]
logos loop complete [run-id] --result <value> [--cwd .]
logos loop abort [run-id] --reason <text> [--result <value>] [--cwd .]
logos loop reflect <run-id> --content <value> [--cwd .]
logos loop ps                       # 查看当前 page 的活跃 run（不含 current 子命令）
logos loop history|show ...
```

### 分层构建

Irollfile 支持四条指令：

```dockerfile
FROM base-agent          # 继承基础层
MIGRATE add_field.sql        # 执行 SQL 迁移，写入 inner 蓝图
MIGRATE OUTER add_field.sql  # 执行 SQL 迁移，写入 outer 模板
COPY logo.png img/           # 复制资源文件到 Resources/
```

## 数据库结构

### roll-inner.db / roll-outer.db（每个 iroll 包）

打开时 outer 为主库，`ATTACH roll-inner.db AS inner`；裸表名指向 outer，访问 inner 表需带 `inner.` 前缀。`schema_version` 当前为 2。

**roll-inner.db（只读蓝图，构建时写入）**

| 表 | 说明 |
|----|------|
| metadata | key-value 元数据 |
| dna | 决策基因（name, type, question, answer, weight） |
| loop | 可复用行为种子（name, type[auto/normal], content, weight, describe, archived_at） |
| skill | skill 定义与内容 |
| book | Book Bundle 元数据和资源路径 |
| history | 构建历史 |
| 模板行 | pages/memory/loop_runs 中 page_id='0' 的模板行 |

**roll-outer.db（每个 page 复制一份模板）**

| 表 | 说明 |
|----|------|
| pages | 页面上下文（page_id, context, alias） |
| memory | page 隔离的问答式记忆（name, question, content, importance, sleep_count） |
| loop_runs | page 独立的运行状态与不可变生命记录（每行 page_id） |

### system.db（全局）

| 表 | 说明 |
|----|------|
| page_index | 页面索引（含 iroll_version, outer_db_path, alias 列） |
| active_page | 每个 cwd 的活跃页面（含 iroll_version, outer_db_path 列） |
| config | 全局配置 |

## 项目结构

```
ai-logos/
├── logos                     # 编译产物
├── iroll/                    # Go 源码
│   ├── book/                 # Book Bundle 校验与检索
│   ├── builder/              # Irollfile 分层构建
│   ├── cmd/                  # Cobra 命令实现
│   ├── db/                   # SQLite 数据操作
│   ├── safepath/             # 路径安全校验
│   ├── store/                # 存储管理
│   └── go.mod
├── irollhub/                 # .iroll 注册中心 HTTP 服务（独立 Go module）
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

Logos CLI 已接入 irollhub，支持 `login/logout/push/pull/search`。Hub 端使用 SQLite FTS5 做搜索，构建和测试 irollhub 时需要启用 `sqlite_fts5` Go build tag。

## 输出格式

高频命令（`page` / `loop` / `evolving` / `query-dna` / `memory`）采用**三段式 JSON** 输出：

- 成功：`{"status":"ok"}`，可附带 `data` 字段，可附带 `{"hints":[...]}` 提示。
- 失败：`{"status":"error","code":...,"error":...}`，可附带 `hints`，并以 exit 1 退出。

注意：`book` / `status` / `skill` / `roll*` 仍使用旧的单行格式，尚未统一为三段式。

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

### irollhub 构建与测试

```bash
cd irollhub
go build -tags sqlite_fts5 .
go test -tags sqlite_fts5 ./...
go run -tags sqlite_fts5 . config.yaml
```

## License

MIT
