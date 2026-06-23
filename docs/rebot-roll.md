# Logos — AI Agent 状态与知识管理系统

## 1. 项目定位

Logos 是一个 AI agent 的状态与知识管理工具。它提供标准化的 `.iroll`（智能卷轴）包格式，用于存储 agent 的人格、记忆、运行循环、知识和资源。通过 `logos` 命令行工具，agent 可以加载、构建、管理和共享这些包。

**核心原则：里面不集成任何 agent 的能力。让 agent 用我们。**

## 2. 包格式 .iroll

`.iroll` 是一个 ZIP 归档文件，包含两部分：

```
agent.iroll (ZIP)
├── Resources/              # 资产：图片、音频、文本、脚本、技能文件等
│   ├── scripts/
│   ├── skills/
│   └── books/
└── ai_roll.db              # SQLite 数据库，包含 agent 的全部记忆和知识
```

## 3. 数据库结构 ai_roll.db

数据库分为四个部分，字段不限制，可自由扩展。

### 3.1 自我部分

| 表 | 必须 | 说明 |
|----|------|------|
| metadata | 是 | key-value 元数据（name, version, description 等） |
| dna | 否 | agent 的决策 DNA，Q&A 对定义底层决策机制 |
| loop | 否 | agent 可自主选择的可复用行为种子 |
| loop_runs | 否 | page 独立的运行状态与生命记录 |

**metadata 表结构：**

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| key | TEXT | NOT NULL | 键 |
| value | TEXT | NOT NULL | 值 |
| remark | TEXT | | 备注 |
| created_at | TEXT | NOT NULL | 创建时间 |
| updated_at | TEXT | NOT NULL | 更新时间 |

**dna 表结构：**

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PRIMARY KEY AUTOINCREMENT | 主键 |
| name | TEXT | NOT NULL | 唯一标识，如 `handle-correction` |
| type | TEXT | NOT NULL | 决策维度：认知观 / 伦理观 / 审美观 / 本体观 |
| question | TEXT | NOT NULL | 决策困境 |
| answer | TEXT | NOT NULL | 这个 agent 的选择 |
| weight | REAL | DEFAULT 0.5 | 权重，越高越核心 |
| created_at | TEXT | NOT NULL | 创建时间 |
| updated_at | TEXT | NOT NULL | 更新时间 |

**loop 表结构（循环种子）：**

loop 表定义循环任务的「种子」——即任务的静态定义。每次执行时会创建一条 `loop_runs` 记录，快照种子的 name/describe/content/weight。

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PRIMARY KEY AUTOINCREMENT | 主键 |
| name | TEXT | NOT NULL UNIQUE | 稳定短标识，如 `self-cognition` |
| describe | TEXT | NOT NULL | 简短描述，如 "自我认知" |
| content | TEXT | NOT NULL | 完整行为种子 |
| weight | REAL | 0..1 | agent 选择时的优先级参考 |
| archived_at | TEXT | | 非空表示归档 |
| created_at | TEXT | NOT NULL | 创建时间 |
| updated_at | TEXT | NOT NULL | 更新时间 |

`loop_runs` 是运行事实的唯一来源，包含 `page_id`、可选的一层 `parent_run_id`、种子快照、`plan/progress/result/reflection` 和 `active/completed/aborted` 状态。每个 page 最多一个 active 主 run，不同 page 可以同时运行同一种子。

Logos 只管理上下文和记录，不执行任务。读取 page context 时会动态注入 `loop.focus` 与 `loop.available`，原始 `pages.context` 不保存 loop 运行状态。

### 3.2 记忆部分

| 表 | 说明 |
|----|------|
| memory | 记忆存储 |
| forget | 遗忘机制（待定义） |
| pages | 页面上下文 |

**memory 表结构（参考 dna 表的 Q&A 设计）：**

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PRIMARY KEY AUTOINCREMENT | 主键 |
| page_id | TEXT | NOT NULL | 所属页面 |
| name | TEXT | NOT NULL | 索引名，如 `user-name-preference` |
| question | TEXT | NOT NULL | 提出什么问题能触发这条记忆 |
| content | TEXT | NOT NULL | 记忆的具体内容（回答） |
| importance | REAL | NOT NULL DEFAULT 0.5 | 重要度 0.0-1.0 |
| sleep_count | INTEGER | NOT NULL DEFAULT 0 | 已被 sleep 整理的次数 |
| created_at | TEXT | NOT NULL | 创建时间 |
| updated_at | TEXT | NOT NULL | 更新时间（sleep 整理时更新） |

**pages 表结构：**

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PRIMARY KEY AUTOINCREMENT | 主键 |
| page_id | TEXT | NOT NULL | 页面唯一 ID（UUID） |
| cwd | TEXT | | 工作目录 |
| context | TEXT | NOT NULL | 页面上下文（JSON 格式） |
| created_at | TEXT | NOT NULL | 创建时间 |
| updated_at | TEXT | NOT NULL | 更新时间 |

**模板页面：** `page_id='0'` 的记录存储默认上下文，新建页面时自动继承模板的 `context` 数据。

**context 字段格式：** 一个 JSON 对象，key 为自定义字段名，value 支持三种类型：

| 类型 | 格式 | 说明 | 示例 |
|------|------|------|------|
| 纯字符串 | `"key": "value"` | 直接存储字符串值 | `"system_prompt": "你是一个AI助手"` |
| 文件引用 | `"key": {"@file": "path"}` | 相对 iroll 包根目录的文件路径，读取时解析为文件内容 | `"greeting": {"@file": "Resources/greeting.txt"}` |
| SQL 查询 | `"key": {"@sql": "SELECT ..."}` | SQL 查询语句，读取时解析为查询结果 | `"description": {"@sql": "SELECT value FROM metadata WHERE key = 'description'"}` |

**解析规则：** `@file` 和 `@sql` 在读取时（`get-context`、`page new`）解析为实际值。写入时（`update-context`）存储原始标记，不做解析。

**loop 自动注入：** `get-context` 和 `page new` 会自动在 context 中注入一个 `loop` 字段，包含当前页面的循环上下文：

```json
{
  "loop": {
    "focus": {
      "main": { "run_id": 1, "seed_name": "self-cognition", "status": "active", "plan": "...", "progress": "..." },
      "children": []
    },
    "available": [
      { "id": 1, "name": "self-cognition", "describe": "自我认知", "weight": 0.9, "stats": { "active": 0, "completed": 5, "aborted": 0 } },
      { "id": 2, "name": "daily-check", "describe": "日常检查", "weight": 0.8, "stats": { "active": 0, "completed": 3, "aborted": 0 } }
    ]
  }
}
```

- `focus`：当前页面活跃的运行（主运行 + 子运行）
- `available`：所有未归档的种子及其运行统计

### 3.3 知识部分

| 表 | 说明 |
|----|------|
| book | 已注册 Book Bundle 的元数据与资源路径 |
| skill | 技能元数据（构建时从 Resources/skills/ 自动发现并注册） |

Book Bundle 的内容存储在 `Resources/books/<book-id>/`，SQLite `book` 表仅保存用于列举、检查和定位资源的元数据。每个 Bundle 必须包含：

```text
Resources/books/<book-id>/
├── manifest.json
├── chunks.parquet
├── inverted_index.parquet
└── idf_stats.parquet
```

构建时会完整校验 Bundle 并同步 `book` 表。查询时由 agent 提供精确标签，Logos 返回原文片段和可解释评分；Logos 不提取标签，也不生成答案。完整格式见 [Book Search Design](superpowers/specs/2026-06-09-book-search-design.md)。

### 3.4 其他部分

| 表 | 说明 |
|----|------|
| history | 构建历史（分层构建时自动维护） |

## 4. 工作目录 ~/.iroll/

所有 .iroll 包加载后解压到 `~/.iroll/` 目录下，以包名称为子目录：

```
~/.iroll/
├── system.db              # 全局系统数据库
├── base-agent/            # 已加载的 iroll 包
│   ├── Resources/
│   └── ai_roll.db
└── python-expert/
    ├── Resources/
    └── ai_roll.db
```

### 4.1 系统数据库 system.db

全局管理，不依赖任何单个 iroll 包：

| 表 | 说明 |
|----|------|
| page_index | 所有页面的索引（iroll_name, page_id, cwd） |
| active_page | 按工作目录追踪活跃页面（每个 cwd 一条记录） |
| config | 全局配置 key-value |

**page_index 表结构：**

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PRIMARY KEY AUTOINCREMENT | 主键 |
| iroll_name | TEXT | NOT NULL | 所属 iroll 包名 |
| page_id | TEXT | NOT NULL | 页面 ID |
| cwd | TEXT | NOT NULL | 工作目录 |
| created_at | TEXT | NOT NULL | 创建时间 |

**active_page 表结构：**

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PRIMARY KEY AUTOINCREMENT | 主键 |
| cwd | TEXT | NOT NULL UNIQUE | 工作目录（唯一） |
| iroll_name | TEXT | NOT NULL | 活跃页面所属 iroll 包 |
| page_id | TEXT | NOT NULL | 活跃页面 ID |
| updated_at | TEXT | NOT NULL | 更新时间 |

## 5. CLI 命令 logos

系统无需手动初始化，首次使用任何命令时会自动创建 `~/.iroll/` 和 `system.db`。

### 5.1 系统状态

| 命令 | 说明 |
|------|------|
| `logos status` | 查看系统状态（home 路径、iroll 数量、page 数量、roll 列表） |

### 5.2 包管理

| 命令 | 说明 |
|------|------|
| `logos roll build -f <file> -t <name>` | 从 Irollfile 分层构建 iroll |
| `logos roll load <file>` | 加载 .iroll 文件到 ~/.iroll/ |
| `logos roll list` | 列出所有已加载的 iroll |
| `logos roll rm <name>` | 删除一个 iroll 包（同时清理 system.db 中的相关记录） |
| `logos roll save <name> [-o path]` | 将 iroll 打包为 .iroll 文件 |
| `logos roll inspect <name>` | 查看 iroll 详情（metadata、表统计、资源列表） |
| `logos roll history <name>` | 查看构建历史 |

### 5.3 页面管理

| 命令 | 说明 |
|------|------|
| `logos page new <name> [--cwd .]` | 创建新页面（继承模板 page_id='0' 的 context，自动设为当前 cwd 的活跃页面） |
| `logos page current [--cwd .]` | 查看当前活跃页面 |
| `logos page list [name] [--cwd .] [-a]` | 列出页面。不指定 name 查全局索引，`-a` 查所有 cwd |
| `logos page switch <page-id>` | 切换活跃页面 |
| `logos page delete <page-id>` | 删除页面 |
| `logos page get-context [name] [--page <id>] [--cwd .]` | 获取上下文（返回解析后的实际值） |
| `logos page update-context [name] --content <json> [--page <id>] [--cwd .]` | 更新上下文（存储原始 JSON 标记） |
| `logos page query-memory [name] [--keyword <text>] [--min-importance 0.7] [--since <ts>] [--before <ts>] [--limit 20] [--full] [--cwd .]` | 检索当前 page 的记忆；默认摘要，`--full` 返回完整内容 |
| `logos page query-dna <name-keyword> [--type <type>] [--cwd .]` | 按名称模糊查询 DNA，可按维度过滤 |

**省略模式：** `page new` 后自动设为活跃页面，后续命令可省略 name 和 --page，自动使用当前 cwd 的活跃页面。

### 5.4 分层构建

**Irollfile 指令（仅三条）：**

| 指令 | 格式 | 说明 |
|------|------|------|
| FROM | `FROM <name>` | 指定基础层（本地 ~/.iroll/ 下已有的包） |
| MIGRATE | `MIGRATE <file.sql>` | 执行 SQL（建表、改字段、插数据） |
| COPY | `COPY <src> <dest>` | 复制文件到 Resources/ |

构建完成所有 Irollfile 指令后，会自动发现、校验并注册 `Resources/books/` 下的 Book Bundle。任何无效 Bundle 都会使构建失败。

### 5.5 Loop

```bash
logos loop list [--archived] [--cwd .]
logos loop add <name> --describe <text> --content <text> [--weight 0.5] [--cwd .]
logos loop edit|inspect|remove|archive|restore ...
logos loop run <name> [--parent <main-run-id>] [--plan <json-or-text>] [--cwd .]
logos loop update [run-id] [--plan <value>] [--progress <value>] [--cwd .]
logos loop complete [run-id] --result <value> [--cwd .]
logos loop abort [run-id] --reason <text> [--result <value>] [--cwd .]
logos loop reflect <run-id> --content <value> [--cwd .]
logos loop current|history|show ...
```

省略 `update/complete/abort` 的 run ID 时，命令作用于当前 page 的 active 主 run。子 run 必须显式指定。主 run 有 active 子 run 时不能结束。

### 5.6 知识书籍

| 命令 | 说明 |
|------|------|
| `logos book list [name] [--cwd .]` | 列出 iroll 中已注册的书籍 |
| `logos book inspect <book-id> [name] [--cwd .]` | 查看书籍元数据 |
| `logos book query --book <id>... --tag <tag>... [--limit 10] [--per-book-limit 5] [--cwd .]` | 按精确标签检索书籍原文片段 |

`book query` 使用当前 cwd 的活跃 iroll。`--book` 与 `--tag` 可重复传入；标签会去除首尾空白、将英文转为小写并去重。

### 5.7 技能

| 命令 | 说明 |
|------|------|
| `logos skill list [name] [--cwd .]` | 列出 iroll 中已注册的技能（name、description、weight、abs_path） |
| `logos skill show <skill-name> [name] [--cwd .]` | 查看单个技能详情（含 skill.md 绝对路径，agent 自行读取内容） |

## 6. 技术栈

- Go 1.24, Cobra CLI 框架
- SQLite（go-sqlite3, CGO）
- Parquet（parquet-go）

## 7. 路线图

### 已完成

- [x] .iroll 包格式定义（ZIP + SQLite）
- [x] system.db 全局页面索引 + 按 cwd 追踪活跃页面
- [x] CLI 命令体系（status / roll / page / loop / book）
- [x] context 标准化格式（纯字符串 / @file / @sql 三种值类型，读时解析）
- [x] 模板页面（page_id='0'）继承机制
- [x] 页面管理（new / current / list / switch / delete / get-context / update-context / query-dna）
- [x] 分层构建（FROM / MIGRATE / COPY）
- [x] 构建历史追踪
- [x] dna 表（决策 DNA：认知观/伦理观/审美观/本体观）
- [x] loop 种子、page 独立 loop_runs、动态 context 与 CLI
- [x] memory 重构、page 隔离与 query-memory CLI
- [x] 路径安全校验（iroll 名称、资源路径、ZIP 解压、符号链接）
- [x] Book Bundle v1（Parquet 校验、构建注册、多书标签检索）

### 待做

- [ ] 遗忘表定义 + 实现
- [x] skill 表 + Resources/skills/ 技能管理（构建时发现、校验、注册，CLI 查询）
- [ ] context 压缩写入 memory
- [ ] Logos CLI 接入 irollhub
- [ ] 前端界面

## 8. 记忆生命周期

### 8.1 context 溢出 → memory 快照

本节描述尚未实现的 context 压缩目标。页面 context 随对话增长，当超过阈值时触发快照：

1. **快照**：将当前完整 context 作为一条 memory 存入，importance 由当时的上下文重要性决定
2. **压缩**：page 的 context 被压缩为摘要，保留关键结构
3. **循环**：压缩后的 context 继续增长，再次超阈值时重复快照 → 压缩

```
context 增长 → 超阈值 → 快照存入 memory → context 压缩为摘要 → 继续增长 → ...
```

memory 表中会积累多轮快照，每条代表一个历史阶段。

### 8.2 sleep 循环 — 记忆整理

`sleep` 是未来可由 agent 自主选择的 loop 种子。Logos 不会自动调度它；agent 启动 run 后可遍历 memory 并执行整理：

1. **提取核心结构**：从每条 memory 中提炼最重要的信息，保留在 memory 中（原地更新或替换为精简版）
2. **遗忘次要细节**：将不重要的部分移入 forget 表，标记遗忘原因和时间

```
memory ──sleep 整理──→ memory（精简的核心）+ forget（被遗忘的细节）
```

**forget 表（待定义）将存储：** 被遗忘的内容原文、来源 memory ID、遗忘原因、遗忘时间。数据不删除，只是从活跃记忆中移出，需要时可检索恢复。

### 8.3 更新后的表结构

**memory 表结构（更新，参考 dna Q&A 设计）：**

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PRIMARY KEY AUTOINCREMENT | 主键 |
| page_id | TEXT | NOT NULL | 所属页面 |
| name | TEXT | NOT NULL | 索引名，如 `user-prefers-python-312` |
| question | TEXT | NOT NULL | 提出什么问题能触发这条记忆 |
| content | TEXT | NOT NULL | 记忆的具体内容（回答，sleep 整理后可能被替换为精简版） |
| importance | REAL | NOT NULL DEFAULT 0.5 | 重要度 0.0-1.0 |
| sleep_count | INTEGER | NOT NULL DEFAULT 0 | sleep 整理次数 |
| created_at | TEXT | NOT NULL | 创建时间 |
| updated_at | TEXT | NOT NULL | 更新时间（sleep 整理时更新） |

**forget 表结构（提案，尚未实现）：**

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PRIMARY KEY AUTOINCREMENT | 主键 |
| memory_id | INTEGER | NOT NULL FK → memory(id) | 来源记忆 |
| page_id | TEXT | NOT NULL | 所属页面 |
| content | TEXT | NOT NULL | 被遗忘的原始内容 |
| reason | TEXT | | 遗忘原因 |
| forgotten_at | TEXT | NOT NULL | 遗忘时间 |

## 9. 技能系统 skill

### 9.1 概念定位

在整个 Logos 体系中，各模块各司其职：

| 模块 | 本质 | 触发方式 |
|------|------|----------|
| dna | 性格 | 被动（决策时参考） |
| loop | 习惯 | 自主（agent 从 context 选择） |
| memory | 经历 | 被动（回忆时检索） |
| book | 知识 | 被动（提问时查阅） |
| **skill** | **能力** | **按需（匹配时调用）** |

Skill 是 agent 可调用的能力单元。和 loop 的区别：loop 描述 agent 可以自主追求的持续行为，skill 描述完成具体工作的能力。二者都由 agent 主动选择，Logos 不负责执行。

### 9.2 文件结构

每个 skill 存储在 `Resources/skills/<skill-name>/` 目录下：

```text
Resources/skills/<skill-name>/
├── skill.md          # 必需：技能定义（名称、触发描述、指令）
├── scripts/          # 可选：附带脚本
└── references/       # 可选：参考文档
```

**skill.md 格式：**

```markdown
---
name: skill-name
description: 触发描述，说明什么时候应该使用这个技能
---

# 技能标题

具体的指令内容，告诉 agent 如何执行这个能力。
可以引用 scripts/ 和 references/ 中的资源。
```

### 9.3 skill 表结构

构建时扫描 `Resources/skills/`，校验每个 skill.md 并注册到 skill 表：

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PRIMARY KEY AUTOINCREMENT | 主键 |
| name | TEXT | NOT NULL UNIQUE | 技能标识，对应目录名 |
| description | TEXT | NOT NULL | 触发描述（从 skill.md frontmatter 读取） |
| path | TEXT | NOT NULL | skill.md 相对路径，如 `Resources/skills/my-skill/skill.md` |
| weight | REAL | NOT NULL DEFAULT 0.5 | 优先级权重 |
| archived_at | TEXT | | 归档时间 |
| created_at | TEXT | NOT NULL | 创建时间 |
| updated_at | TEXT | NOT NULL | 更新时间 |

### 9.4 工作流程

```
构建时：Resources/skills/ → 校验 skill.md → 注册 skill 表
使用时：agent 读取 skill 列表 → 匹配当前情境 → 加载对应 skill.md → 执行
```

1. **注册**：构建 iroll 时自动扫描 `Resources/skills/`，校验每个目录下必须有 `skill.md` 且 frontmatter 包含 name 和 description，通过后写入 skill 表
2. **发现**：agent 通过 `logos skill list` 查询 skill 列表，获取 name、description 和 abs_path
3. **匹配**：agent 根据 description 判断当前情境需要哪个 skill
4. **加载**：读取对应 skill.md 的完整内容，按指令执行
5. **执行**：可调用 scripts/ 中的脚本，参考 references/ 中的文档

### 9.5 与 context 的集成

skill 的 description 可通过 `@sql` 注入到 context 中，让 agent 在每次对话开始时就了解自己有哪些能力：

```json
{
  "skills": {"@sql": "SELECT name, description FROM skill WHERE archived_at IS NULL ORDER BY weight DESC"}
}
```

未来 agent 可以在 context 中看到技能列表，根据用户请求匹配对应的 skill，再按需加载完整指令。
