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
- 构建过程是 append-only：追加数据，不删除基础层数据
- 每层携带 schema migration，处理表结构演进
- Resources 目录按文件路径覆盖（与 Docker 层行为一致）
- Layerfile 只保留三条核心指令，所有数据操作统一通过 SQL 完成

---

## 2. 包格式

### 2.1 基础 .iroll 包（无分层）

与现有格式完全兼容：

```
base-agent.iroll (ZIP)
├── Resources/
│   ├── avatar.png
│   └── greeting.txt
└── ai_roll.db
    ├── metadata        (key-value 元数据)
    ├── memory          (记忆)
    ├── context         (会话上下文)
    ├── skill           (技能)
    ├── knowledge_chunk (知识分块)
    └── ...
```

### 2.2 层文件 .iroll.layer

一个层是一个轻量的 .iroll 包，只包含**本层新增或变更的内容**：

```
python-expert.layer.iroll (ZIP)
├── Resources/
│   ├── scripts/
│   │   └── python_linter.py      ← 本层新增的脚本
│   └── skills/
│       └── code_review.md         ← 本层新增的技能文件
├── ai_roll.db                     ← 只包含本层新增的数据
│   ├── memory (3 rows)            ← Python 相关记忆
│   ├── skill  (2 rows)            ← 技能注册
│   └── ...
└── layer.json                     ← 层描述文件
```

### 2.3 layer.json 规范

每个层必须包含 `layer.json`，描述本层元信息：

```json
{
  "layer_id": "sha256:a1b2c3d4...",
  "parent": "sha256:e5f6g7h8...",
  "description": "添加 Python 专家知识和代码审查技能",
  "created_at": "2026-06-04T08:00:00Z",
  "author": "community",
  "schema_version": 1,
  "migration": "migration.sql",
  "resources_hash": "sha256:i9j0k1l2..."
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| layer_id | string | 是 | 本层内容 SHA256 哈希 |
| parent | string | 否 | 父层哈希，基础层为 null |
| description | string | 是 | 层描述 |
| created_at | string | 是 | 创建时间 ISO 8601 |
| author | string | 否 | 创建者 |
| schema_version | int | 是 | 目标 schema 版本号 |
| migration | string | 否 | schema 变更 SQL 文件名 |
| resources_hash | string | 否 | Resources 目录内容哈希 |

---

## 3. 层定义文件 Layerfile

类似 `Dockerfile`，iroll 使用 `Layerfile` 定义如何构建一层。

### 3.1 设计理念

Layerfile 只保留三条指令：**FROM、MIGRATE、COPY**。

所有数据库操作（建表、改字段、插入记忆、注册技能、设置元数据）统一通过 MIGRATE 执行 SQL 文件完成。不单独提供 MEMORY、SKILL、META 等指令，原因：

- SQL 本身已经能完成所有数据操作（CREATE TABLE、ALTER TABLE、INSERT、UPDATE）
- 减少指令数量，降低解析器复杂度
- 用户对 SQL 有完整控制力，不受指令语法限制
- 一份 SQL 文件可以用任何方式生成（手写、脚本、导出工具）

### 3.2 指令说明

| 指令 | 格式 | 说明 |
|------|------|------|
| FROM | `FROM <name>[:<tag>]` | 指定基础层，本地 `~/.iroll/` 下已有的 iroll 包 |
| MIGRATE | `MIGRATE <file>` | 执行 SQL 文件（schema 变更 + 数据插入） |
| COPY | `COPY <src> <dest>` | 复制文件到 Resources 目录 |

### 3.3 Layerfile 示例

```layerfile
# 从基础 agent 开始
FROM base-agent:v0.1.0

# 执行 schema 变更和数据插入
MIGRATE schema/v2_add_tables.sql
MIGRATE data/python_knowledge.sql
MIGRATE data/project_meta.sql

# 复制资源文件
COPY scripts/ Resources/scripts/
COPY skills/ Resources/skills/
COPY docs/ Resources/docs/
```

---

## 4. 构建流程

### 4.1 整体流程

```
Layerfile
    │
    ▼
逐行解析指令
    │
    ▼
FROM ──── 复制基础层到临时构建目录
    │
    ▼
MIGRATE ── 读取 SQL 文件 → 在 ai_roll.db 上执行
    │         （可包含 CREATE TABLE / ALTER TABLE / INSERT / UPDATE 等）
    │
    ▼
COPY ───── 复制宿主机文件到 Resources/（同名覆盖）
    │
    ▼
生成 layer.json
    │
    ▼
INSERT INTO history ── 记录构建历史
    │
    ▼
打包为 .iroll → load 到 ~/.iroll/
```

### 4.2 详细步骤

**Step 1：FROM — 复制基础层**

```bash
# 从 ~/.iroll/<base-name>/ 复制完整包到临时构建目录
cp -r ~/.iroll/base-agent/ /tmp/iroll-build-xxx/
```

基础层的内容（数据库 + 资源）作为起点，后续所有操作都在这个副本上进行。

无 FROM 的 Layerfile 为基础层构建，此时创建空的 ai_roll.db 和 Resources/ 目录。

**Step 2：MIGRATE — 执行 SQL**

读取 Layerfile 同目录下的 SQL 文件，在构建目录的 `ai_roll.db` 上执行。

SQL 文件可以是任何合法的 SQLite SQL，典型用途：

**Schema 变更：**

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

```sql
-- data/python_knowledge.sql
INSERT INTO memory (content, created_at, importance)
VALUES
    ('Python 3.12 引入了类型参数语法', datetime('now'), 0.8),
    ('GIL 在 Python 3.13 可通过 --disable-gil 关闭', datetime('now'), 0.7),
    ('uv 是 Python 最快的包管理器', datetime('now'), 0.6);

INSERT INTO skill (name, description, file_path, created_at)
VALUES
    ('code_review', '代码审查', 'Resources/skills/code_review.md', datetime('now')),
    ('python_lint', 'Python 静态检查', 'Resources/scripts/python_linter.py', datetime('now'));
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

计算本层内容的 SHA256 哈希，生成 layer.json 写入构建目录。

**Step 5：记录构建历史**

向 ai_roll.db 的 history 表插入记录：

```sql
INSERT INTO history (from_layer, description, layer_id, instructions, created_at)
VALUES ('sha256:e5f6g7h8...', '添加 Python 专家知识', 'sha256:a1b2c3d4...',
        '["MIGRATE v2.sql","COPY scripts/ Resources/scripts/"]',
        '2026-06-04T08:00:00Z');
```

**Step 6：打包并加载**

将构建目录打包为 .iroll 文件，然后 load 到 `~/.iroll/<name>/`。

### 4.3 history 表结构

```sql
CREATE TABLE history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    from_layer TEXT,           -- 父层哈希，基础层为 NULL
    description TEXT NOT NULL,  -- 本层做了什么
    layer_id TEXT NOT NULL,     -- 本层哈希
    instructions TEXT,          -- Layerfile 指令摘要（JSON 数组）
    created_at TEXT NOT NULL
);
```

查询示例：

```bash
logos history my-agent
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

```
~/.iroll/
├── layers/                          # 层缓存
│   ├── sha256:e5f6g7h8.../
│   │   ├── ai_roll.db
│   │   ├── Resources/
│   │   └── layer.json
│   └── sha256:a1b2c3d4.../
│       ├── ai_roll.db
│       ├── Resources/
│       └── layer.json
├── hello-agent/                     # 运行时实例（已合并）
│   ├── ai_roll.db
│   └── Resources/
└── my-python-agent/                 # 另一个运行时实例
    ├── ai_roll.db
    └── Resources/
```

### 5.2 层复用

```
base-agent (sha256:e5f6...)
    ├── python-expert (sha256:a1b2...)  ──→ my-python-agent
    └── legal-expert  (sha256:m5n6...)  ──→ my-legal-agent

python-expert 被多个 agent 共享，不重复存储。
```

### 5.3 Registry（远期）

```
logos push my-agent registry.example.com/python-expert:latest
logos pull registry.example.com/python-expert:latest
```

类似 Docker Hub，可以发布和拉取层。这是远期功能，当前仅做本地。

---

## 6. CLI 命令扩展

在现有 CLI 基础上新增构建相关命令：

### 6.1 build

```bash
logos build -f Layerfile -t my-python-agent
```

| 参数 | 说明 |
|------|------|
| `-f, --file` | Layerfile 路径，默认当前目录 `Layerfile` |
| `-t, --tag` | 输出 iroll 名称 |

流程：
1. 解析 Layerfile
2. 复制 FROM 指定的基础层到临时目录（无 FROM 则创建空包）
3. 按顺序执行 MIGRATE（执行 SQL）和 COPY（复制文件）指令
4. 生成 layer.json 和 history 记录
5. 打包为 .iroll 并 load 到 ~/.iroll/

### 6.2 history

```bash
logos history <name>
```

输出该 iroll 的构建历史（查询 history 表）。

### 6.3 inspect

```bash
logos inspect <name>
```

输出 iroll 的详细元信息：metadata、层链、表统计、资源列表。

---

## 7. 完整示例

### 7.1 基础层：通用助手

```layerfile
# base-agent/Layerfile
# 没有 FROM，这是最底层

MIGRATE init_schema.sql
MIGRATE init_data.sql

COPY avatar.png Resources/avatar.png
COPY personality.md Resources/personality.md
```

`init_schema.sql`：

```sql
CREATE TABLE metadata (
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    remark TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE memory (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    content TEXT NOT NULL,
    created_at TEXT NOT NULL,
    importance REAL DEFAULT 0.5
);

CREATE TABLE context (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    cwd TEXT,
    content TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    from_layer TEXT,
    description TEXT NOT NULL,
    layer_id TEXT NOT NULL,
    instructions TEXT,
    created_at TEXT NOT NULL
);
```

`init_data.sql`：

```sql
INSERT INTO metadata (key, value, remark, created_at, updated_at) VALUES
    ('name', 'base-agent', 'agent 名称', datetime('now'), datetime('now')),
    ('version', '0.1.0', '版本号', datetime('now'), datetime('now')),
    ('description', '通用 AI 助手基础层', '描述', datetime('now'), datetime('now'));

INSERT INTO memory (content, created_at, importance) VALUES
    ('我是一个友好的 AI 助手', datetime('now'), 0.9),
    ('我应该诚实、有帮助、无害', datetime('now'), 0.95);
```

构建：

```bash
logos build -f base-agent/Layerfile -t base-agent
```

### 7.2 中间层：Python 专家

```layerfile
# python-expert/Layerfile
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
INSERT INTO memory (content, created_at, importance) VALUES
    ('Python 3.12 引入了类型参数语法', datetime('now'), 0.8),
    ('GIL 在 Python 3.13 可通过 --disable-gil 关闭', datetime('now'), 0.7),
    ('uv 是 Python 最快的包管理器', datetime('now'), 0.6);
```

`data/python_skills.sql`：

```sql
INSERT INTO skill (name, description, file_path, created_at) VALUES
    ('code_review', '代码审查', 'Resources/skills/code_review.md', datetime('now')),
    ('python_lint', 'Python 静态检查', 'Resources/scripts/python_linter.py', datetime('now'));
```

`data/python_meta.sql`：

```sql
INSERT INTO metadata (key, value, remark, created_at, updated_at) VALUES
    ('domain', 'python', '专业领域', datetime('now'), datetime('now')),
    ('expertise_level', 'expert', '熟练度', datetime('now'), datetime('now'));
```

构建：

```bash
logos build -f python-expert/Layerfile -t python-expert
```

### 7.3 应用层：我的项目助手

```layerfile
# my-project/Layerfile
FROM python-expert

MIGRATE data/project_knowledge.sql
MIGRATE data/project_meta.sql

COPY docs/ Resources/docs/
```

`data/project_knowledge.sql`：

```sql
INSERT INTO memory (content, created_at, importance) VALUES
    ('这个项目使用 Go 和 Python', datetime('now'), 0.8),
    ('数据库使用 SQLite，格式为 .iroll', datetime('now'), 0.7);
```

`data/project_meta.sql`：

```sql
INSERT INTO metadata (key, value, remark, created_at, updated_at) VALUES
    ('project', 'ai-roll-mini', '项目名', datetime('now'), datetime('now'));
```

构建：

```bash
logos build -f my-project/Layerfile -t my-project
```

### 7.4 查看历史

```bash
$ logos history my-project
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

现有 CLI 命令保持不变，新增命令为增量：

| 现有命令 | 说明 |
|----------|------|
| `logos load <file>` | 加载 .iroll（不涉及分层） |
| `logos list` | 列出已加载的 iroll |
| `logos session init/list` | 管理 context |
| `logos get-context / update-context` | 读写 context |
| `logos add-memory` | 新增记忆 |

| 新增命令 | 说明 |
|----------|------|
| `logos build -f Layerfile -t <name>` | 分层构建 |
| `logos history <name>` | 查看构建历史 |
| `logos inspect <name>` | 查看详细信息 |

load 命令加载任何 .iroll（无论是否经过分层构建），build 产出的 .iroll 可以通过 load 分享给其他人。

---

## 9. 约束与边界

- **不做层合并冲突解决**：构建是 append-only，不删除基础层数据
- **不做远端 Registry**：当前仅本地构建，远期扩展
- **不做层回滚**：history 只读，不提供 undo
- **不做跨数据库 JOIN**：每层合并到同一个 ai_roll.db，不存在跨库查询
- **Layerfile 只有三条指令**：FROM / MIGRATE / COPY，不引入更多指令
