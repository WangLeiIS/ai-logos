# iroll 分层构建技术文档

## 1. 概述

### 1.1 什么是 iroll

iroll（intelligent roll，智能卷轴）是一个 AI agent 的便携包格式。后缀 `.iroll`，本质为 ZIP 归档文件，包含一个 agent 的全部状态：记忆、技能、知识和资源。

### 1.2 为什么需要分层

当前每个 .iroll 是一个独立的完整包。如果 10 个 agent 都需要"Python 专家"的基础知识和"法律合规"的技能，这些内容会被复制 10 次。

分层构建借鉴 Docker 镜像的思想：

- **基础层**包含通用人格和世界知识，可以被所有 agent 共享
- **中间层**叠加领域知识、技能、脚本，按需组合
- **顶层**是 agent 的运行时状态（context、近期记忆），可丢弃

核心收益：**复用、组合、版本化**。

### 1.3 设计原则

- 每一层本身也是一个合法的 .iroll 包
- FROM 指令整库复制基础层（roll-inner.db + roll-outer.db + Resources/），后续 MIGRATE 在副本上追加或修改数据
- 每层携带 schema migration，处理表结构演进
- Resources 目录按文件路径覆盖（与 Docker 层行为一致）
- Irollfile 提供四条指令（FROM / MIGRATE / MIGRATE OUTER / COPY），所有数据操作统一通过 SQL 完成

---

## 2. 包格式

### 2.1 基础 .iroll 包（无分层）

与现有格式完全兼容：

```
base-agent.iroll (ZIP)
├── Resources/
│   ├── avatar.png
│   └── greeting.txt
├── roll-inner.db       (inner 库：记忆、技能、DNA、loop、pages 等)
├── roll-outer.db       (outer 库：模板/共享数据，与 inner 分离)
└── layer.json          (层描述文件)
```

说明：

- `metadata`、`memory`、`pages`（其 `context` 列存放会话上下文）、`skill`、`book` 等表位于 roll-inner.db 或 roll-outer.db，并非单一的 `ai_roll.db`。
- 不存在独立的 `context` 表 —— 会话上下文是 `pages.context` 列。
- `history` 表由代码（`db.EnsureHistoryTable`）在构建时自动创建，不需要用户在 SQL 里建。

### 2.2 层的数据库来源

当前实现不做"按层 diff/append"的增量打包。`FROM` 触发的 `processFrom` 会把基础层**整个目录**（roll-inner.db、roll-outer.db、Resources/、layer.json）复制到临时构建目录，之后所有的 MIGRATE 都直接作用在这个完整的副本上：

```
临时构建目录（FROM python-expert 之后）
├── Resources/                     ← 整库继承自基础层，COPY 按路径覆盖
│   ├── scripts/
│   │   └── python_linter.py
│   └── skills/
│       └── code_review.md
├── roll-inner.db                  ← 基础层的完整 inner 库副本
│                                    MIGRATE 在其上 CREATE TABLE / INSERT / UPDATE
├── roll-outer.db                  ← 基础层的完整 outer 库副本
│                                    MIGRATE OUTER 在其上执行
└── layer.json                     ← 构建末尾被覆写为本层的描述
```

因此每一层产出的 .iroll 都是自包含的完整包，而不是只含本层增量的轻量包。

### 2.3 layer.json 规范

每个层必须包含 `layer.json`，描述本层元信息。实际写入的字段如下（对应 `builder.LayerJSON` 结构）：

```json
{
  "layer_id": "sha256:a1b2c3d4...",
  "parent": "sha256:e5f6g7h8...",
  "description": "build from Irollfile for python-expert:latest",
  "created_at": "2026-06-04T08:00:00Z",
  "schema_version": 2
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| layer_id | string | 是 | 本层构建目录内容的 SHA256 哈希 |
| parent | string | 否 | 父层 layer_id（来自基础层的 layer.json），基础层为空，省略 |
| description | string | 是 | 层描述（自动生成：`build from Irollfile for <name>:<version>`） |
| created_at | string | 是 | 创建时间 RFC3339Nano |
| schema_version | int | 是 | 当前固定为 2 |

注意：早期设计稿里出现过的 `author` / `migration` / `resources_hash` 字段**当前实现并未写入**，不要在生产 SQL 或工具里依赖它们。

---

## 3. 层定义文件 Irollfile

类似 `Dockerfile`，iroll 使用 `Irollfile` 定义如何构建一层。

### 3.1 设计理念

Irollfile 提供四条指令：**FROM、MIGRATE、MIGRATE OUTER、COPY**。

所有数据库操作（建表、改字段、插入记忆、注册技能、设置元数据）统一通过 MIGRATE / MIGRATE OUTER 执行 SQL 文件完成。不单独提供 MEMORY、SKILL、META 等指令，原因：

- SQL 本身已经能完成所有数据操作（CREATE TABLE、ALTER TABLE、INSERT、UPDATE）
- 减少指令数量，降低解析器复杂度
- 用户对 SQL 有完整控制力，不受指令语法限制
- 一份 SQL 文件可以用任何方式生成（手写、脚本、导出工具）

### 3.2 指令说明

| 指令 | 格式 | 说明 |
|------|------|------|
| FROM | `FROM <name>[:<tag>]` | 指定基础层，本地 `~/.iroll/<name>/<version>/` 下已有的 iroll 包；整库复制 inner/outer/Resources 到构建目录 |
| MIGRATE | `MIGRATE <file>` | 执行 SQL 文件到 **roll-inner.db**（schema 变更 + 数据插入） |
| MIGRATE OUTER | `MIGRATE OUTER <file>` | 执行 SQL 文件到 **roll-outer.db**（模板/共享层） |
| COPY | `COPY <src> <dest>` | 复制宿主机文件到 Resources 目录 |

指令按 Irollfile 中的书写顺序依次执行。

### 3.3 Irollfile 示例

```layerfile
# 从基础 agent 开始
FROM base-agent:v0.1.0

# 执行 schema 变更和数据插入（写入 roll-inner.db）
MIGRATE schema/v2_add_tables.sql
MIGRATE data/python_knowledge.sql
MIGRATE data/project_meta.sql

# 执行 outer 库的共享模板数据（写入 roll-outer.db）
MIGRATE OUTER data/shared_templates.sql

# 复制资源文件
COPY scripts/ Resources/scripts/
COPY skills/ Resources/skills/
COPY docs/ Resources/docs/
```

---

## 4. 构建流程

### 4.1 整体流程

```
Irollfile
    │
    ▼
逐行解析指令（按书写顺序）
    │
    ▼
FROM ──── processFrom：整库复制基础层到临时构建目录
    │         （roll-inner.db + roll-outer.db + Resources/）
    ▼
MIGRATE ── 读取 SQL 文件 → 在 roll-inner.db 上执行
    │         （可包含 CREATE TABLE / ALTER TABLE / INSERT / UPDATE 等）
    │
    ▼
MIGRATE OUTER ── 读取 SQL 文件 → 在 roll-outer.db 上执行
    │
    ▼
COPY ───── 复制宿主机文件到 Resources/（同名覆盖）
    │
    ▼
Discover books/skills → EnsureHistoryTable → 生成 layer.json
    │
    ▼
INSERT INTO history ── 记录构建历史（history 表由代码自动创建）
    │
    ▼
复制到 ~/.iroll/<name>/<version>/ 并更新 latest 指针
```

### 4.2 详细步骤

**Step 1：FROM — 整库复制基础层**

```bash
# processFrom：把 ~/.iroll/<base-name>/<base-version>/ 整个目录复制到临时构建目录
cp -r ~/.iroll/base-agent/v0.1.0/ /tmp/iroll-build-xxx/
# 复制后临时目录里已经有 roll-inner.db、roll-outer.db、Resources/、layer.json
```

基础层的完整内容（两个数据库 + 资源 + 旧 layer.json）作为起点，后续所有 MIGRATE / COPY 都在这个副本上进行。旧 layer.json 会在构建末尾被本层的 layer.json 覆写。

无 FROM 的 Irollfile 为基础层构建，此时构建目录里只有空的 roll-inner.db / roll-outer.db 和 Resources/ 目录，schema 由后续 MIGRATE 建立。

**Step 2：MIGRATE / MIGRATE OUTER — 执行 SQL**

读取 Irollfile 同目录下的 SQL 文件。`MIGRATE` 在构建目录的 `roll-inner.db` 上执行；`MIGRATE OUTER` 在 `roll-outer.db` 上执行。

SQL 文件可以是任何合法的 SQLite SQL，典型用途：

**Schema 变更（MIGRATE → inner）：**

```sql
-- schema/v2_add_tables.sql
CREATE TABLE IF NOT EXISTS knowledge_chunk (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    content TEXT NOT NULL,
    source TEXT,
    embedding BLOB,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS knowledge_index (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    chunk_id INTEGER NOT NULL,
    token TEXT NOT NULL,
    FOREIGN KEY (chunk_id) REFERENCES knowledge_chunk(id)
);

ALTER TABLE memory ADD COLUMN tags TEXT DEFAULT '';
```

**数据插入（记忆、技能、元数据）：**

> 注意：`memory` 表要求 `page_id` / `name` / `question` 三列 NOT NULL，缺失会插入失败；`skill` 表的资源路径列名是 `path`（不是 `file_path`）。下面是合法写法：

```sql
-- data/python_knowledge.sql
INSERT INTO memory (page_id, name, question, content, importance, sleep_count, created_at, updated_at)
VALUES
    ('0', 'py-typeparams', 'Python 3.12 有什么新语法？', 'Python 3.12 引入了类型参数语法', 0.8, 0, datetime('now'), datetime('now')),
    ('0', 'py-no-gil',     '怎么关闭 GIL？',             'GIL 在 Python 3.13 可通过 --disable-gil 关闭', 0.7, 0, datetime('now'), datetime('now')),
    ('0', 'py-uv',         '最快的 Python 包管理器？',   'uv 是 Python 最快的包管理器', 0.6, 0, datetime('now'), datetime('now'));

-- skill 注册：path 指向 Resources 下的文件，weight 默认 0.5
INSERT INTO skill (name, description, path, weight, archived_at, created_at, updated_at)
VALUES
    ('code_review', '代码审查',       'Resources/skills/code_review.md',        0.5, NULL, datetime('now'), datetime('now')),
    ('python_lint', 'Python 静态检查', 'Resources/scripts/python_linter.py',    0.5, NULL, datetime('now'), datetime('now'));
```

```sql
-- data/project_meta.sql
INSERT INTO metadata (key, value, remark, created_at, updated_at)
VALUES
    ('domain', 'python', '专业领域', datetime('now'), datetime('now')),
    ('expertise_level', 'expert', '熟练度', datetime('now'), datetime('now'));
```

**Step 3：COPY — 复制资源**

将宿主机文件复制到构建目录的 Resources/ 下，按路径覆盖：

```
宿主机 scripts/python_linter.py → Resources/scripts/python_linter.py
宿主机 skills/code_review.md    → Resources/skills/code_review.md
```

如果基础层已有同名文件，直接覆盖（与 Docker 层行为一致）。

**Step 4：生成 layer.json**

计算临时构建目录内容的 SHA256 哈希作为 `layer_id`，结合 `parent`（来自基础层 layer.json 的 layer_id）、`description`、`created_at`、`schema_version=2`，生成 layer.json 写入构建目录。注意 layer.json 和 history 属于构建元数据，不计入 layer_id 哈希。

**Step 5：记录构建历史**

代码先调用 `db.EnsureHistoryTable` 确保 `history` 表存在（无需用户在 SQL 中手动建表），再向 roll-inner.db 的 history 表插入记录：

```sql
INSERT INTO history (from_layer, description, layer_id, instructions, created_at)
VALUES ('sha256:e5f6g7h8...', '添加 Python 专家知识', 'sha256:a1b2c3d4...',
        '["MIGRATE v2.sql","COPY scripts/ Resources/scripts/"]',
        '2026-06-04T08:00:00Z');
```

**Step 6：复制到存储**

将构建目录整体复制到 `~/.iroll/<name>/<version>/`，并更新 `~/.iroll/<name>/latest` 指针（成功时建 symlink，权限不足时回退写 `.latest` 文件）。注意：**当前实现不会打包成 .iroll 再 load**，而是直接落盘到目标目录；如需 .iroll 归档文件请单独打包。

### 4.3 history 表结构

```sql
CREATE TABLE history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    from_layer TEXT,           -- 父层哈希，基础层为 NULL
    description TEXT NOT NULL,  -- 本层做了什么
    layer_id TEXT NOT NULL,     -- 本层哈希
    instructions TEXT,          -- Irollfile 指令摘要（JSON 数组）
    created_at TEXT NOT NULL
);
```

查询示例：

```bash
logos roll history my-agent
```

输出：

```json
[
  {
    "id": 1,
    "from_layer": null,
    "description": "初始化基础 agent",
    "layer_id": "sha256:e5f6g7h8...",
    "created_at": "2026-06-04T06:00:00Z"
  },
  {
    "id": 2,
    "from_layer": "sha256:e5f6g7h8...",
    "description": "添加 Python 专家知识",
    "layer_id": "sha256:a1b2c3d4...",
    "created_at": "2026-06-04T08:00:00Z"
  },
  {
    "id": 3,
    "from_layer": "sha256:a1b2c3d4...",
    "description": "添加法律合规技能",
    "layer_id": "sha256:m5n6o7p8...",
    "created_at": "2026-06-04T10:00:00Z"
  }
]
```

---

## 5. 层的存储与共享

### 5.1 本地存储结构

构建产物直接落盘到 `~/.iroll/<name>/<version>/`，每个 iroll 拥有自己的目录树：

```
~/.iroll/
├── base-agent/
│   └── v0.1.0/                       # 一个完整的 iroll（自包含）
│       ├── roll-inner.db
│       ├── roll-outer.db
│       ├── Resources/
│       └── layer.json
├── python-expert/                    # FROM base-agent 整库复制后再 MIGRATE 的产物
│   └── latest/                       # latest 是指向当前版本目录的 symlink（或 .latest 文件回退）
│       ├── roll-inner.db
│       ├── roll-outer.db
│       ├── Resources/
│       └── layer.json
└── my-python-agent/                  # FROM python-expert 的产物
    └── latest/
        ├── ...
```

### 5.2 关于"层复用"的说明

> 历史设计稿曾设想基于内容哈希的层缓存（`~/.iroll/layers/<sha256>/`）与跨 agent 去重。**当前实现并未提供这套机制**：每次 `logos roll build` 都会把基础层整库复制一份，再独立落盘到该 iroll 自己的目录。因此即便多个 agent 都 FROM 同一个基础层，它们各自的 roll-inner.db / roll-outer.db 仍是完整的物理副本，不会共享存储。

`layer.json` 里的 `layer_id` / `parent` 仅用于追溯构建谱系（history 表），并不驱动任何去重或缓存逻辑。

### 5.3 Registry（远期）

通过 irollhub（HTTP 注册中心）发布与拉取，地址格式 `org/pkg:ver`：

```bash
# 把已构建的 iroll（按 name 或直接指向 .iroll 文件）推送到 irollhub
logos roll push my-python-agent org/python-expert:latest
# 或推一个本地归档文件
logos roll push ./my-python-agent.iroll org/python-expert:latest
```

irollhub 是独立模块（`irollhub/`），按 组织 → 包 → 版本 三级组织，并提供 OAuth → API Key 认证与全文搜索。这是远期能力，当前主线以本地构建为主。

---

## 6. CLI 命令扩展

构建相关命令挂在 `logos roll` 子命令下：

### 6.1 build

```bash
logos roll build -f Irollfile -t my-python-agent
# 等价写法（默认读当前目录的 Irollfile）：
logos roll build -t my-python-agent
# -t 支持 name:version 形式，省略 version 时默认为 latest
logos roll build -f path/to/Irollfile -t my-python-agent:v0.2.0
```

| 参数 | 说明 |
|------|------|
| `-f, --file` | Irollfile 路径，默认当前目录 `Irollfile` |
| `-t, --tag` | 输出 iroll 名称（必填），可带 `:version` |

流程：
1. 解析 Irollfile
2. FROM：整库复制基础层到临时目录（无 FROM 则用空库）
3. 按书写顺序执行 MIGRATE（→ roll-inner.db）/ MIGRATE OUTER（→ roll-outer.db）/ COPY 指令
4. Discover books/skills、EnsureHistoryTable、生成 layer.json 和 history 记录
5. 复制到 `~/.iroll/<name>/<version>/` 并更新 latest 指针

### 6.2 history

```bash
logos roll history <name>           # 默认 latest
logos roll history my-python-agent:v0.2.0
```

输出该 iroll 的构建历史（查询 roll-inner.db 的 history 表）。

### 6.3 inspect

```bash
logos inspect <name>
```

输出 iroll 的详细元信息：metadata、层链、表统计、资源列表。

---

## 7. 完整示例

### 7.1 基础层：通用助手

```layerfile
# base-agent/Irollfile
# 没有 FROM，这是最底层

MIGRATE init_schema.sql
MIGRATE init_data.sql

COPY avatar.png Resources/avatar.png
COPY personality.md Resources/personality.md
```

`init_schema.sql`：

```sql
CREATE TABLE IF NOT EXISTS metadata (
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    remark TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- memory 表：page_id / name / question 三列为 NOT NULL，content/importance/sleep_count/timestamps 由应用读写
CREATE TABLE IF NOT EXISTS memory (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    page_id TEXT NOT NULL,
    name TEXT NOT NULL,
    question TEXT NOT NULL,
    content TEXT NOT NULL,
    importance REAL DEFAULT 0.5,
    sleep_count INTEGER DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- pages 表：会话上下文是 pages.context 列，不存在独立的 context 表
CREATE TABLE IF NOT EXISTS pages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    page_id TEXT NOT NULL,
    cwd TEXT,
    context TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- 注意：history 表由代码（db.EnsureHistoryTable）在构建时自动创建，不要在 init_schema.sql 里手写。
```

`init_data.sql`：

```sql
INSERT INTO metadata (key, value, remark, created_at, updated_at) VALUES
    ('name', 'base-agent', 'agent 名称', datetime('now'), datetime('now')),
    ('version', '0.1.0', '版本号', datetime('now'), datetime('now')),
    ('description', '通用 AI 助手基础层', '描述', datetime('now'), datetime('now'));

-- 记忆必须给全 page_id / name / question / content；模板页 page_id 约定为 '0'
INSERT INTO memory (page_id, name, question, content, importance, sleep_count, created_at, updated_at) VALUES
    ('0', 'persona',   '你是什么样的助手？', '我是一个友好的 AI 助手',        0.9, 0, datetime('now'), datetime('now')),
    ('0', 'principle', '你的行为准则？',     '我应该诚实、有帮助、无害',      0.95, 0, datetime('now'), datetime('now'));
```

构建：

```bash
logos roll build -f base-agent/Irollfile -t base-agent
```

### 7.2 中间层：Python 专家

```layerfile
# python-expert/Irollfile
FROM base-agent

MIGRATE schema/v2_add_knowledge_tables.sql
MIGRATE data/python_knowledge.sql
MIGRATE data/python_skills.sql
MIGRATE data/python_meta.sql

COPY scripts/python_linter.py Resources/scripts/python_linter.py
COPY skills/code_review.md Resources/skills/code_review.md
```

`data/python_knowledge.sql`：

```sql
-- 注意：必须给 page_id / name / question，否则 NOT NULL 约束会失败
INSERT INTO memory (page_id, name, question, content, importance, sleep_count, created_at, updated_at) VALUES
    ('0', 'py-typeparams', 'Python 3.12 有什么新语法？', 'Python 3.12 引入了类型参数语法', 0.8, 0, datetime('now'), datetime('now')),
    ('0', 'py-no-gil',     '怎么关闭 GIL？',             'GIL 在 Python 3.13 可通过 --disable-gil 关闭', 0.7, 0, datetime('now'), datetime('now')),
    ('0', 'py-uv',         '最快的 Python 包管理器？',   'uv 是 Python 最快的包管理器', 0.6, 0, datetime('now'), datetime('now'));
```

`data/python_skills.sql`：

```sql
-- skill 表的资源路径列名是 path（不是 file_path）；weight 默认 0.5
INSERT INTO skill (name, description, path, weight, archived_at, created_at, updated_at) VALUES
    ('code_review', '代码审查',        'Resources/skills/code_review.md',     0.5, NULL, datetime('now'), datetime('now')),
    ('python_lint', 'Python 静态检查', 'Resources/scripts/python_linter.py',  0.5, NULL, datetime('now'), datetime('now'));
```

`data/python_meta.sql`：

```sql
INSERT INTO metadata (key, value, remark, created_at, updated_at) VALUES
    ('domain', 'python', '专业领域', datetime('now'), datetime('now')),
    ('expertise_level', 'expert', '熟练度', datetime('now'), datetime('now'));
```

构建：

```bash
logos roll build -f python-expert/Irollfile -t python-expert
```

### 7.3 应用层：我的项目助手

```layerfile
# my-project/Irollfile
FROM python-expert

MIGRATE data/project_knowledge.sql
MIGRATE data/project_meta.sql

COPY docs/ Resources/docs/
```

`data/project_knowledge.sql`：

```sql
INSERT INTO memory (page_id, name, question, content, importance, sleep_count, created_at, updated_at) VALUES
    ('0', 'proj-stack', '本项目用什么技术栈？', '这个项目使用 Go 和 Python',       0.8, 0, datetime('now'), datetime('now')),
    ('0', 'proj-db',    '数据库是什么格式？',   '数据库使用 SQLite，格式为 .iroll', 0.7, 0, datetime('now'), datetime('now'));
```

`data/project_meta.sql`：

```sql
INSERT INTO metadata (key, value, remark, created_at, updated_at) VALUES
    ('project', 'ai-roll-mini', '项目名', datetime('now'), datetime('now'));
```

构建：

```bash
logos roll build -f my-project/Irollfile -t my-project
```

### 7.4 查看历史

```bash
$ logos roll history my-project
```

```json
[
  {"id": 1, "from_layer": null, "description": "初始化基础 agent", "layer_id": "sha256:e5f6..."},
  {"id": 2, "from_layer": "sha256:e5f6...", "description": "添加 Python 专家知识", "layer_id": "sha256:a1b2..."},
  {"id": 3, "from_layer": "sha256:a1b2...", "description": "项目级定制", "layer_id": "sha256:m5n6..."}
]
```

---

## 8. 与现有 CLI 的关系

构建命令与日常运行时命令是同一套 CLI 的不同子树：

| 命令 | 说明 |
|------|------|
| `logos load <file>` | 加载一个 .iroll 归档到 `~/.iroll/`（不涉及分层） |
| `logos list` | 列出已加载的 iroll |
| `logos status` / `logos version` | 查看 store 状态 / CLI 版本 |
| `logos inspect <name>` | 查看 iroll 详细元信息 |
| `logos page new <iroll-name>` | 在当前工作目录创建一个 page（继承模板页） |
| `logos page list -a` | 列出所有 page |
| `logos page get [--page <id>]` | 读取 page 的完整解析后 context（含 DNA、loop 注入等） |
| `logos page set <path> <value>` / `--content '<json>'` | 修改 page 的某个 context 键 |
| `logos page query-memory [--keyword <k>]` | 按 keyword 检索 memory 全文（无手动 add-memory，记忆由 agent 运行时写入） |
| `logos evolving [name:version] [sql]` | 在模板层（roll-outer.db + roll-inner.db）执行 ad-hoc SQL |

| 构建相关命令 | 说明 |
|----------|------|
| `logos roll build -f <Irollfile> -t <name>` | 按 Irollfile 分层构建 |
| `logos roll history <name>` | 查看构建历史 |
| `logos roll push <file\|name> <org>/<pkg>:<ver>` | 推送到 irollhub（远期） |

> 说明：早期设计稿里提到过的 `logos session init/list`、`logos get-context / update-context`、`logos add-memory` 等命令**当前实现并不存在**；context 的读写由 `logos page get / page set` 完成，记忆的写入由 agent 在运行时通过 page 机制完成，没有手动 add-memory 命令。

---

## 9. 约束与边界

- **不做层合并冲突解决**：FROM 整库复制基础层，MIGRATE 在副本上追加或修改数据，不做冲突仲裁
- **不做内容寻址层缓存**：当前构建直接落盘到 `~/.iroll/<name>/<version>/`，没有 `~/.iroll/layers/<sha256>/` 形式的去重缓存，跨 agent 不共享存储
- **不做远端 Registry**：本地构建为主，`logos roll push` 依赖独立的 irollhub 服务
- **不做层回滚**：history 只读，不提供 undo
- **不跨数据库 JOIN**：roll-inner.db 与 roll-outer.db 物理分离，MIGRATE / MIGRATE OUTER 分别作用于各自库
- **Irollfile 共有四条指令**：FROM / MIGRATE / MIGRATE OUTER / COPY，不引入更多指令
