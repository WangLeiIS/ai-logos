# Logos — AI Agent 状态与知识管理系统

## 1. 项目定位

Logos 是一个 AI agent 的状态与知识管理工具。它提供标准化的 `.iroll`（智能卷轴）包格式，用于存储 agent 的人格、记忆、运行循环、知识和资源。通过 `logos` 命令行工具，agent 可以加载、构建、管理和共享这些包。

**核心原则：里面不集成任何 agent 的能力。让 agent 用我们。**

## 2. 包格式 .iroll

`.iroll` 是一个 ZIP 归档文件，包含 `roll-inner.db`、`roll-outer.db`、`Resources/` 和构建元数据 `layer.json`：

```
agent.iroll (ZIP)
├── Resources/              # 资产：图片、音频、文本、脚本、技能文件等
│   ├── scripts/
│   ├── skills/
│   └── books/
├── roll-inner.db          # 只读蓝图库（构建时写入）：metadata/dna/loop/skill/book/history + 模板行
├── roll-outer.db          # 工作库模板：pages/memory/loop_runs 的 schema（运行时按 cwd 复制）
└── layer.json             # 构建层元数据（layer_id、parent、schema_version 等）
```

两个数据库的分工见 §3。

## 3. 数据库结构

`.iroll` 包内有两个 SQLite 数据库，分工不同：

- **`roll-inner.db`（只读蓝图，构建时写入）**：存放 roll 级的稳定定义——`metadata` / `dna` / `loop`（种子）/ `skill` / `book` / `history`，以及 `pages` / `memory` / `loop_runs` 中 `page_id='0'` 的模板行。所有 inner 表通过 `inner.` 前缀访问。
- **`roll-outer.db`（工作库模板）**：只含 `pages` / `memory` / `loop_runs` 三张表的 schema。加载或 `page new` 时按 cwd 复制一份作为实际工作库：`<cwd>/.iroll/<name>.db`，或默认工作区 `~/.iroll/<name>/<version>/workspace/.<name>.outer.db`。

运行时打开方式：以 outer 为主库，`ATTACH` inner 为 `inner.` schema。裸表名（`pages`/`memory`/`loop_runs`）指向 outer；带 `inner.` 前缀的指向蓝图。因此 `@sql` 引用蓝图表时必须写成 `inner.metadata`、`inner.loop` 等。

字段不限制，可自由扩展。下文按功能划分介绍。

### 3.1 自我部分

| 表 | 必须 | 说明 |
|----|------|------|
| metadata | 是 | key-value 元数据（name, version, description 等） |
| dna | 否 | agent 的决策 DNA，Q&A 对定义底层决策机制 |
| loop | 否 | agent 可自主选择的可复用行为种子（roll 级） |

> 以上三张表都在 `roll-inner.db`（蓝图表）。`loop_runs` 虽属「自我」范畴，但属于 page 工作数据，见 §3.2。

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
| type | TEXT | NOT NULL | 自由字符串，标注这条基因的取向（示例：`idea`、`emotion`），不做枚举约束 |
| question | TEXT | NOT NULL | 决策困境 |
| answer | TEXT | NOT NULL | 这个 agent 的选择 |
| weight | REAL | DEFAULT 0.5 | 权重，越高越核心 |
| created_at | TEXT | NOT NULL | 创建时间 |
| updated_at | TEXT | NOT NULL | 更新时间 |

**loop 表结构（循环种子，位于 `roll-inner.db`）：**

loop 表定义循环任务的「种子」——即任务的静态定义（roll 级，所有 page 共享）。每次执行时在 `loop_runs`（outer）中创建一条记录，快照种子的 name/describe/content/weight。

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PRIMARY KEY AUTOINCREMENT | 主键 |
| name | TEXT | NOT NULL UNIQUE | 稳定短标识，如 `self-cognition` |
| type | TEXT | NOT NULL DEFAULT 'normal' CHECK IN ('auto','normal') | `auto` 在 `page new` 时自动启动 run；`normal` 由 agent 主动选择 |
| describe | TEXT | NOT NULL | 简短描述，如 "自我认知" |
| content | TEXT | NOT NULL | 完整行为种子 |
| weight | REAL | 0..1 | agent 选择时的优先级参考 |
| archived_at | TEXT | | 非空表示归档 |
| created_at | TEXT | NOT NULL | 创建时间 |
| updated_at | TEXT | NOT NULL | 更新时间 |

`loop_runs` 表存运行事实，包含 `page_id`、可选的一层 `parent_run_id`、种子快照、`plan/progress/result/reflection` 和 `active/completed/aborted` 状态。每个 page 最多一个 active 主 run，不同 page 可以同时运行同一种子。该表 schema 同时存在于 inner（模板行）和 outer（实际运行数据，见 §3.2）。

Logos 只管理上下文和记录，不执行任务。读取 page context 时会动态注入顶层键 `loop_focus` 与 `loop_available`，原始 `pages.context` 不保存 loop 运行状态（详见 §3.2）。

### 3.2 记忆部分

| 表 | 说明 |
|----|------|
| memory | 记忆存储（page 隔离） |
| pages | 页面上下文 |
| loop_runs | 循环运行记录（page 隔离） |
| forget | 遗忘机制（未实现，仅 §8 设计提案） |

> 这三张工作表（`memory` / `pages` / `loop_runs`）的 schema 同时存在于 `roll-inner.db`（含 `page_id='0'` 模板行）和 `roll-outer.db`（实际运行时数据）。运行时读写的是 outer 库的副本。

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
| alias | TEXT | | 页面别名，可用 `--alias` 引用（见 §5.3） |
| context | TEXT | NOT NULL | 页面上下文（JSON 格式） |
| created_at | TEXT | NOT NULL | 创建时间 |
| updated_at | TEXT | NOT NULL | 更新时间 |

**模板页面：** `page_id='0'` 的记录存储默认上下文，新建页面时自动继承模板的 `context` 数据。

**context 字段格式：** 一个 JSON 对象，key 为自定义字段名，value 支持三种类型：

| 类型 | 格式 | 说明 | 示例 |
|------|------|------|------|
| 纯字符串 | `"key": "value"` | 直接存储字符串值 | `"system_prompt": "你是一个AI助手"` |
| 文件引用 | `"key": {"@file": "path"}` | 相对 iroll 包根目录的文件路径，读取时解析为文件内容 | `"greeting": {"@file": "Resources/greeting.txt"}` |
| SQL 查询 | `"key": {"@sql": "SELECT ..."}` | SQL 查询语句，读取时解析为查询结果 | `"description": {"@sql": "SELECT value FROM inner.metadata WHERE key = 'description'"}` |

**解析规则：** `@file` 和 `@sql` 在读取时（`page get`、`page new`）解析为实际值。写入时（`page set`）存储原始标记，不做解析。`@sql` 默认对 outer 主库执行；要查 inner 蓝图表（如 metadata / dna / loop / skill / book）必须加 `inner.` 前缀，例如 `SELECT value FROM inner.metadata WHERE key = 'description'`。

**loop 自动注入：** `page get` 和 `page new` 会自动在 context 顶层注入两个键，反映当前页面的循环上下文：

```json
{
  "loop_focus": [
    { "run_id": 1, "seed_name": "self-cognition", "status": "active", "plan": "...", "progress": "..." }
  ],
  "loop_available": [
    { "id": 2, "name": "daily-check", "describe": "日常检查", "weight": 0.8, "stats": { "active": 0, "completed": 3, "aborted": 0 } }
  ]
}
```

- `loop_focus`：当前页面活跃的运行列表（主运行 + 子运行）
- `loop_available`：所有未归档且 `type='normal'` 的种子及其运行统计（`type='auto'` 的种子由 `page new` 自动启动，不在可选列表中重复出现）

### 3.3 知识部分

| 表 | 说明 |
|----|------|
| book | 已注册 Book Bundle 的元数据与资源路径（位于 `roll-inner.db`） |
| skill | 技能元数据，构建时从 `Resources/skills/` 自动发现并注册（位于 `roll-inner.db`） |

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
| history | 构建历史（分层构建时自动维护，位于 `roll-inner.db`） |

## 4. 工作目录 ~/.iroll/

所有 .iroll 包加载后解压到 `~/.iroll/` 目录下，按 `名称/版本` 两级存放：

```
~/.iroll/
├── system.db                      # 全局系统数据库（page_index / active_page / config）
├── base-agent/
│   └── latest/                    # 版本目录（默认 latest；可多版本并存）
│       ├── Resources/             # 资产目录
│       ├── roll-inner.db          # 只读蓝图库
│       ├── roll-outer.db          # 工作库模板
│       ├── layer.json             # 构建层元数据
│       └── workspace/             # 默认工作区
│           └── .base-agent.outer.db   # 该工作区 page 的 outer 库副本
└── python-expert/
    └── latest/
        ├── Resources/
        ├── roll-inner.db
        ├── roll-outer.db
        └── layer.json
```

自定义 cwd 的 page 会把 outer 库复制到 `<cwd>/.iroll/<name>.db`，不走 `workspace/`。

### 4.1 系统数据库 system.db

全局管理，不依赖任何单个 iroll 包：

| 表 | 说明 |
|----|------|
| page_index | 所有页面的索引（iroll_name、版本、page_id、cwd、outer 路径、alias） |
| active_page | 按工作目录追踪活跃页面（每个 cwd 一条记录） |
| config | 全局配置 key-value（如 `default_page:<name>` 记录每个 iroll 的默认页面，供 `page default` 使用） |

**page_index 表结构：**

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PRIMARY KEY AUTOINCREMENT | 主键 |
| iroll_name | TEXT | NOT NULL | 所属 iroll 包名 |
| iroll_version | TEXT | NOT NULL DEFAULT 'latest' | 所属版本 |
| page_id | TEXT | NOT NULL | 页面 ID |
| cwd | TEXT | NOT NULL | 工作目录 |
| outer_db_path | TEXT | NOT NULL DEFAULT '' | 该 page 对应的 outer 库绝对路径 |
| alias | TEXT | | 页面别名（可空） |
| created_at | TEXT | NOT NULL | 创建时间 |

**active_page 表结构：**

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PRIMARY KEY AUTOINCREMENT | 主键 |
| cwd | TEXT | NOT NULL UNIQUE | 工作目录（唯一） |
| iroll_name | TEXT | NOT NULL | 活跃页面所属 iroll 包 |
| iroll_version | TEXT | NOT NULL DEFAULT 'latest' | 所属版本 |
| page_id | TEXT | NOT NULL | 活跃页面 ID |
| outer_db_path | TEXT | NOT NULL DEFAULT '' | 该 page 对应的 outer 库绝对路径 |
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
| `logos roll evolving [name:version] [sql]` | 在已存在 iroll 上增量执行 SQL（不重新构建） |
| `logos roll login [--hub <url>]` / `logos roll logout` | 登录 / 登出 irollhub（换取或清除 API Key） |
| `logos roll push <file.iroll\|name> <org>/<pkg>:<ver>` | 推送 .iroll 到 irollhub |
| `logos roll pull <org>/<pkg>[:<ver>] [-o <path>]` | 从 irollhub 拉取 .iroll |
| `logos roll search <keyword> [--tag <tag>]` | 全文检索 irollhub 上的包 |

### 5.3 页面管理

| 命令 | 说明 |
|------|------|
| `logos page new <name> [--cwd .]` | 创建新页面（继承模板 page_id='0' 的 context，自动设为当前 cwd 的活跃页面） |
| `logos page list [name] [--cwd .] [-a]` | 列出页面。不指定 name 查全局索引，`-a` 查所有 cwd |
| `logos page switch <page-id>` | 切换活跃页面 |
| `logos page delete <page-id>` | 删除页面 |
| `logos page default <page-id>` | 设置某 iroll 的默认页面（写入 `config.default_page:<name>`） |
| `logos page default --roll <name> [--clear]` | 查看或清除某 iroll 的默认页面 |
| `logos page get [path] [--page <id>] [--alias <name>] [--roll <tag>] [--cwd .]` | 获取上下文（全量或单键，已解析） |
| `logos page set <path> <value> [--page <id>] [--alias <name>] [--cwd .]` | 设置一个 context 键（json-or-text） |
| `logos page set --content '<json>' [--page <id>] [--cwd .]` | 整体替换 context（存储原始 JSON 标记） |
| `logos page unset <path> [--page <id>] [--alias <name>] [--cwd .]` | 删除一个 context 键 |
| `logos page alias <name> [--page <id>]` | 设置/清除别名（`--clear` 清空） |
| `logos page query [sql] [--sql <stmt>] [--file <p>] [--alias <name>] [--roll <tag>] [--cwd .]` | 对当前 page 的 outer 库跑 SQL |
| `logos page query-memory [name] [--keyword <text>] [--min-importance 0.7] [--since <ts>] [--before <ts>] [--limit 20] [--full] [--cwd .]` | 检索当前 page 的记忆；默认摘要，`--full` 返回完整内容 |
| `logos page query-dna <name-keyword> [--type <type>] [--cwd .]` | 按名称模糊查询 DNA，可按 type 过滤 |

**定位方式：** 多数命令支持 `--page <id>`、`--alias <name>`、`--roll <org/name:ver>` 或省略（用当前 cwd 活跃页面）四种方式定位目标 page。**省略模式：** `page new` 后自动设为活跃页面，后续命令可省略 name 和 --page，自动使用当前 cwd 的活跃页面。

### 5.4 分层构建

**Irollfile 指令（四条，按文件中出现顺序执行）：**

| 指令 | 格式 | 说明 |
|------|------|------|
| FROM | `FROM <name:version>` | 指定基础层（本地 ~/.iroll/ 下已有的包，先把其内容复制到构建临时目录） |
| MIGRATE | `MIGRATE <file.sql>` | 对 **inner** 库（`roll-inner.db`）执行 SQL（建蓝图表、改字段、插种子/模板数据） |
| MIGRATE OUTER | `MIGRATE OUTER <file.sql>` | 对 **outer** 库（`roll-outer.db`）执行 SQL（建工作表 schema，如 pages/memory/loop_runs） |
| COPY | `COPY <src> <dest>` | 复制文件或目录到包内指定位置（通常放在 `Resources/` 下） |

指令按 Irollfile 中的书写顺序逐条执行；`FROM` 一般放第一条，后续 `MIGRATE` / `MIGRATE OUTER` / `COPY` 可穿插。构建完成后会自动发现、校验并注册 `Resources/books/` 下的 Book Bundle 与 `Resources/skills/` 下的技能。任何无效 Bundle 或技能都会使构建失败。

### 5.5 Loop

```bash
logos loop list [--archived] [--stats] [--cwd .]
logos loop inspect <name> [--cwd .]
logos loop add <name> --describe <text> --content <text> [--type auto|normal] [--weight 0.5] [--cwd .]
logos loop edit <name> [--type auto|normal] [--describe <text>] [--content <text>] [--weight 0.5] [--cwd .]
logos loop remove <name> [--cwd .]            # 仅在无运行历史时可删
logos loop archive <name> [--cwd .]
logos loop restore <name> [--cwd .]
logos loop run <name> [--parent <main-run-id>] [--plan <json-or-text>] [--cwd .]
logos loop update [run-id] [--plan <value>] [--progress <value>] [--cwd .]
logos loop complete [run-id] --result <value> [--cwd .]
logos loop abort [run-id] --reason <text> [--result <value>] [--cwd .]
logos loop reflect <run-id> --content <value> [--cwd .]
logos loop ps [-a] [--cwd .]                   # 列出当前 page 的运行（默认仅 active，-a 含已完成/中止）
logos loop history <name> [--page <page-id>] [--limit 50] [--cwd .]
logos loop show <run-id> [--cwd .]
```

- `loop add --type`：`auto` 种子在 `page new` 时自动启动一条 run；`normal`（默认）由 agent 主动 `loop run`。
- `loop list --stats`：附带每个种子的运行统计（active/completed/aborted 计数）。
- `loop ps` 替代了早期的 `loop current`，列出当前 page 的运行记录。
- 省略 `update/complete/abort` 的 run ID 时，命令作用于当前 page 的 active 主 run。子 run 必须显式指定。主 run 有 active 子 run 时不能结束。

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

- [x] .iroll 包格式定义（ZIP + 双 SQLite：roll-inner.db / roll-outer.db）
- [x] system.db 全局页面索引 + 按 cwd 追踪活跃页面（含版本、outer 路径、alias）
- [x] CLI 命令体系（status / roll / page / loop / book / skill）
- [x] context 标准化格式（纯字符串 / @file / @sql 三种值类型，读时解析；@sql 查 inner 需 `inner.` 前缀）
- [x] 模板页面（page_id='0'）继承机制
- [x] 页面管理（new / list / switch / delete / default / get / set / unset / alias / query / query-dna）
- [x] 分层构建（FROM / MIGRATE / MIGRATE OUTER / COPY 四条指令，按文件顺序执行）
- [x] 构建历史追踪
- [x] dna 表（决策 DNA，type 为自由字符串）
- [x] loop 种子（含 type=auto/normal）、page 独立 loop_runs、动态 context 注入（loop_focus / loop_available）与 CLI（含 `loop ps`）
- [x] memory 重构、page 隔离与 query-memory CLI
- [x] 路径安全校验（iroll 名称、资源路径、ZIP 解压、符号链接）
- [x] Book Bundle v1（Parquet 校验、构建注册、多书标签检索）
- [x] skill 表 + Resources/skills/ 技能管理（构建时发现、校验、注册，skill list/show CLI）
- [x] irollhub 接入（roll login / logout / push / pull / search）

### 待做

- [ ] 遗忘表定义 + 实现（见 §8）
- [ ] context 压缩写入 memory（见 §8）
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
