# .iroll Protocol v1

.iroll（智能卷轴）是一个 AI agent 人格与状态包格式。它把 agent 的性格、习惯、记忆、知识和能力封装进一个 ZIP 文件，做到可构建、可分享、可演进。

**版本**: v1  
**最后更新**: 2026-06-11  
**状态**: 冻结

## 1. 文件格式

```
agent.iroll (ZIP)
├── layer.json               # 构建元数据
├── ai_roll.db                # 人格与状态数据库 (SQLite)
└── Resources/                # 资源资产
    ├── books/<book-id>/      # Book Bundle 目录
    ├── skills/<skill-name>/  # Skill 目录
    ├── scripts/              # 通用脚本
    └── ...                   # 其他任意资源文件
```

**约束**:
- .iroll 必须是有效的 ZIP 归档
- 必须包含 `ai_roll.db`
- `Resources/` 下的文件名和路径不受限制，但不能通过符号链接逃逸出包根目录

### 1.1 layer.json

构建时自动生成，记录构建溯源信息：

```json
{
  "layer_id": "sha256:<64-char hex>",
  "parent": "sha256:<64-char hex> or empty",
  "description": "build from Layerfile for <name>",
  "created_at": "2026-06-11T10:00:00Z",
  "schema_version": 1
}
```

- `layer_id`: 当前层的内容哈希（不含 layer.json 和 history），唯一标识一个构建产物
- `parent`: 构建时的基础层 layer_id，如果是 FROM 构建
- `schema_version`: .iroll 协议版本，当前固定为 1

---

## 2. ai_roll.db 数据库

ai_roll.db 是 .iroll 的核心，包含 7 张用户创建的表和 3 张自动维护的表。

### 2.1 用户创建的表（构建时通过 MIGRATE SQL 定义）

---

#### metadata — 元数据

**必须存在**。任意 key-value 对，至少需要 `name`、`version`、`description`。

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| key | TEXT | NOT NULL | 键 |
| value | TEXT | NOT NULL | 值 |
| remark | TEXT | | 备注 |
| created_at | TEXT | NOT NULL | ISO 8601 时间 |
| updated_at | TEXT | NOT NULL | ISO 8601 时间 |

**保留 key**: `name`、`version`、`description` 用于 irollhub 索引和 `roll inspect` 展示。

```sql
CREATE TABLE metadata (
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    remark TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
```

---

#### dna — 决策 DNA

**可选**。Q&A 对定义 agent 在决策困境下的选择倾向。

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | |
| name | TEXT | NOT NULL | 唯一标识，如 `handle-correction` |
| type | TEXT | NOT NULL | 决策维度：认知观 / 伦理观 / 审美观 / 本体观 |
| question | TEXT | NOT NULL | 决策困境 |
| answer | TEXT | NOT NULL | agent 的选择 |
| weight | REAL | DEFAULT 0.5 | 0.0-1.0，越高越核心 |
| created_at | TEXT | NOT NULL | |
| updated_at | TEXT | NOT NULL | |

```sql
CREATE TABLE dna (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    question TEXT NOT NULL,
    answer TEXT NOT NULL,
    weight REAL DEFAULT 0.5,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
```

---

#### loop — 行为种子

**可选**。定义 agent 可以自主选择执行的循环任务。

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | |
| name | TEXT | NOT NULL UNIQUE | 稳定标识，如 `self-cognition` |
| describe | TEXT | NOT NULL | 简短描述，如 "自我认知" |
| content | TEXT | NOT NULL | 完整行为指令 |
| weight | REAL | NOT NULL DEFAULT 0.5, CHECK (0-1) | 优先级参考 |
| archived_at | TEXT | | 非 NULL 表示已归档，不参与运行 |
| created_at | TEXT | NOT NULL | |
| updated_at | TEXT | NOT NULL | |

```sql
CREATE TABLE loop (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    describe TEXT NOT NULL,
    content TEXT NOT NULL,
    weight REAL NOT NULL DEFAULT 0.5 CHECK (weight >= 0 AND weight <= 1),
    archived_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
```

---

#### loop_runs — 运行记录

**可选**（loop 不存在时不需要）。每次执行 loop 种子创建一条记录，追踪完整生命周期。Logos 只做记录，不负责执行。

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | |
| loop_id | INTEGER | NOT NULL FK → loop(id) | 关联的种子 |
| page_id | TEXT | NOT NULL | 所属页面 |
| parent_run_id | INTEGER | FK → loop_runs(id) | 父运行（支持子任务嵌套） |
| seed_name | TEXT | NOT NULL | 启动时快照的种子名 |
| seed_describe | TEXT | NOT NULL | 启动时快照的种子描述 |
| seed_content | TEXT | NOT NULL | 启动时快照的种子指令 |
| seed_weight | REAL | NOT NULL | 启动时快照的种子权重 |
| status | TEXT | NOT NULL, CHECK | active / completed / aborted |
| plan | TEXT | NOT NULL DEFAULT 'null' | 执行计划（JSON） |
| progress | TEXT | NOT NULL DEFAULT 'null' | 执行进度（JSON） |
| result | TEXT | NOT NULL DEFAULT 'null' | 执行结果（JSON） |
| reflection | TEXT | NOT NULL DEFAULT 'null' | 执行反思（JSON，结束后填写） |
| abort_reason | TEXT | | 中止原因 |
| started_at | TEXT | NOT NULL | 启动时间 |
| ended_at | TEXT | | 结束时间 |
| reflected_at | TEXT | | 反思时间 |
| updated_at | TEXT | NOT NULL | |

**约束**:
- 每个 page 最多 1 个 active 主 run (`status='active' AND parent_run_id IS NULL`)
- 主 run 有 active 子 run 时不能结束
- 种子启动时快照 name/describe/content/weight，后续种子变更不影响历史 run

```sql
CREATE TABLE loop_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    loop_id INTEGER NOT NULL,
    page_id TEXT NOT NULL,
    parent_run_id INTEGER,
    seed_name TEXT NOT NULL,
    seed_describe TEXT NOT NULL,
    seed_content TEXT NOT NULL,
    seed_weight REAL NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'completed', 'aborted')),
    plan TEXT NOT NULL DEFAULT 'null',
    progress TEXT NOT NULL DEFAULT 'null',
    result TEXT NOT NULL DEFAULT 'null',
    reflection TEXT NOT NULL DEFAULT 'null',
    abort_reason TEXT,
    started_at TEXT NOT NULL,
    ended_at TEXT,
    reflected_at TEXT,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (loop_id) REFERENCES loop(id),
    FOREIGN KEY (parent_run_id) REFERENCES loop_runs(id)
);

CREATE INDEX idx_loop_runs_page_status ON loop_runs(page_id, status);
CREATE INDEX idx_loop_runs_parent_status ON loop_runs(parent_run_id, status);
CREATE INDEX idx_loop_runs_loop_started ON loop_runs(loop_id, id DESC);
CREATE INDEX idx_loop_runs_loop_ended
    ON loop_runs(loop_id, ended_at DESC, id DESC)
    WHERE status IN ('completed', 'aborted') AND ended_at IS NOT NULL;
CREATE UNIQUE INDEX idx_loop_runs_one_active_main
    ON loop_runs(page_id)
    WHERE status = 'active' AND parent_run_id IS NULL;
```

---

#### memory — 页面记忆

**可选**。按 page_id 隔离的 Q&A 式记忆。记忆由 context 压缩自动写入，不由用户手动添加。

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | |
| page_id | TEXT | NOT NULL | 所属页面 |
| name | TEXT | NOT NULL | 索引名，如 `user-name-preference` |
| question | TEXT | NOT NULL | 提出什么问题能触发这条记忆 |
| content | TEXT | NOT NULL | 记忆内容（回答） |
| importance | REAL | NOT NULL DEFAULT 0.5 | 重要度 0.0-1.0 |
| sleep_count | INTEGER | NOT NULL DEFAULT 0 | sleep 整理的次数 |
| created_at | TEXT | NOT NULL | |
| updated_at | TEXT | NOT NULL | |

```sql
CREATE TABLE memory (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    page_id TEXT NOT NULL,
    name TEXT NOT NULL,
    question TEXT NOT NULL,
    content TEXT NOT NULL,
    importance REAL NOT NULL DEFAULT 0.5,
    sleep_count INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_memory_page ON memory(page_id, importance);
```

---

#### pages — 页面上下文

**必须存在**。存储 page 的身份和动态上下文。

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | |
| page_id | TEXT | NOT NULL | 页面唯一 ID（UUID） |
| cwd | TEXT | | 工作目录 |
| context | TEXT | NOT NULL | 页面上下文（JSON，见第 3 节） |
| created_at | TEXT | NOT NULL | |
| updated_at | TEXT | NOT NULL | |

**模板页面**: `page_id='0'` 的记录是模板 page。创建新页面时继承其 context。

```sql
CREATE TABLE pages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    page_id TEXT NOT NULL,
    cwd TEXT,
    context TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
```

---

#### skill — 能力注册

**可选**。构建时从 `Resources/skills/` 自动发现并注册。

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | |
| name | TEXT | NOT NULL UNIQUE | 技能标识，对应目录名 |
| description | TEXT | NOT NULL | 触发描述（从 skill.md frontmatter 读取） |
| path | TEXT | NOT NULL | skill.md 相对路径 |
| weight | REAL | NOT NULL DEFAULT 0.5 | 优先级 |
| archived_at | TEXT | | 归档时间 |
| created_at | TEXT | NOT NULL | |
| updated_at | TEXT | NOT NULL | |

```sql
CREATE TABLE skill (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL,
    path TEXT NOT NULL,
    weight REAL NOT NULL DEFAULT 0.5,
    archived_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
```

### 2.2 自动维护的表

---

#### book — 已注册的 Book Bundle

构建时从 `Resources/books/` 自动发现、校验并注册。表结构由 logos 自动管理，用户无需在 MIGRATE 中创建。

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | |
| book_id | TEXT | NOT NULL UNIQUE | 书籍标识 |
| title | TEXT | NOT NULL | 书名 |
| description | TEXT | NOT NULL DEFAULT '' | 描述 |
| resource_path | TEXT | NOT NULL | 资源目录路径 |
| format_version | INTEGER | NOT NULL | Book Bundle 格式版本 |
| authors | TEXT | NOT NULL DEFAULT '[]' | 作者列表（JSON 数组） |
| language | TEXT | NOT NULL DEFAULT '' | 语言 |
| tags | TEXT | NOT NULL DEFAULT '[]' | 标签（JSON 数组） |
| created_at | TEXT | NOT NULL | |
| updated_at | TEXT | NOT NULL | |

---

#### history — 构建历史

构建时自动记录，每次构建一行。

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | |
| from_layer | TEXT | | 基础层的 layer_id |
| description | TEXT | NOT NULL | 构建描述 |
| layer_id | TEXT | NOT NULL | 当前层 layer_id |
| instructions | TEXT | | Layerfile 指令摘要（JSON） |
| created_at | TEXT | NOT NULL | |

---

### 2.3 表总结

| 表 | 创建方式 | 必须 | 说明 |
|----|---------|------|------|
| metadata | MIGRATE SQL | 是 | 包元数据 |
| dna | MIGRATE SQL | 否 | 决策 DNA |
| loop | MIGRATE SQL | 否 | 行为种子 |
| loop_runs | MIGRATE SQL | 否 | 运行记录（依赖 loop） |
| memory | MIGRATE SQL | 否 | 页面记忆 |
| pages | MIGRATE SQL | 是 | 页面上下文 |
| skill | MIGRATE SQL | 否 | 能力注册 |
| book | 自动 | — | 书籍注册（构建时从 Resources/books/ 发现） |
| history | 自动 | — | 构建历史 |

---

## 3. Context 格式

`pages.context` 是一个 JSON 对象，key 为自定义字段名，value 支持三种类型：

| 类型 | 格式 | 说明 |
|------|------|------|
| 纯字符串 | `"key": "value"` | 直接存储字符串 |
| 文件引用 | `"key": {"@file": "Resources/..."}` | 读取时解析为文件内容 |
| SQL 查询 | `"key": {"@sql": "SELECT ..."}` | 读取时解析为查询结果 |

**解析规则**: `@file` 和 `@sql` 在读取时（`get-context`、`page new`）解析为实际值。写入时（`update-context`）存储原始标记，不做解析。

**示例**:
```json
{
  "system_prompt": "你是一个AI助手",
  "greeting": {"@file": "Resources/greeting.txt"},
  "description": {"@sql": "SELECT value FROM metadata WHERE key = 'description'"},
  "dna": {"@sql": "SELECT type, weight, question FROM dna ORDER BY weight DESC"},
  "skills": {"@sql": "SELECT name, description FROM skill WHERE archived_at IS NULL ORDER BY weight DESC"}
}
```

### 3.1 Loop 自动注入

`get-context` 和 `page new` 会动态注入一个 `loop` 字段（不在 pages.context 中存储）：

```json
{
  "loop": {
    "focus": {
      "main": { "run_id": 1, "seed_name": "self-cognition", "status": "active", "plan": "...", "progress": "..." },
      "children": []
    },
    "available": [
      { "id": 1, "name": "self-cognition", "describe": "自我认知", "weight": 0.9,
        "stats": { "active": 0, "completed": 5, "aborted": 0 } }
    ]
  }
}
```

- `focus`: 当前 page 活跃的主运行和子运行
- `available`: 所有未归档的种子及其统计

---

## 4. Resources 目录

### 4.1 Book Bundle

存储在 `Resources/books/<book-id>/`：

```
Resources/books/<book-id>/
├── manifest.json        # 书籍元数据
├── chunks.parquet       # 文本块
├── inverted_index.parquet  # 倒排索引
└── idf_stats.parquet    # IDF 统计
```

构建时自动校验完整性（manifest、chunks、index、idf 一致性），校验失败则构建失败。

### 4.2 Skill

存储在 `Resources/skills/<skill-name>/`：

```
Resources/skills/<skill-name>/
├── skill.md          # 必需：能力定义
├── scripts/          # 可选：附带脚本
└── references/       # 可选：参考文档
```

**skill.md 格式**:

```markdown
---
name: skill-name
description: 触发描述
---
# 标题
具体指令...
```

构建时自动扫描、校验（name 与目录名一致、description 非空），通过后注册到 skill 表。校验失败则构建失败。

### 4.3 其他资源

`Resources/` 下可自由存放任意文件。路径通过 `safepath` 校验，禁止符号链接逃逸。

---

## 5. Layerfile 构建指令

仅三条指令：

| 指令 | 格式 | 说明 |
|------|------|------|
| FROM | `FROM <name>` | 继承 ~/.iroll/ 下的现有包 |
| MIGRATE | `MIGRATE <file.sql>` | 执行 SQL（建表、改字段、插数据） |
| COPY | `COPY <src> <dest>` | 复制文件到 Resources/ |

**构建流程**:

1. 处理 Layerfile 指令（FROM → COPY → MIGRATE，逐条执行，遇错停止）
2. 发现并校验 `Resources/books/` → 注册到 book 表
3. 发现并校验 `Resources/skills/` → 注册到 skill 表
4. 计算 layer_id（内容哈希）
5. 写入 layer.json 和 history
6. 输出 .iroll ZIP 包

---

## 6. system.db 全局数据库

存储在 `~/.iroll/system.db`。首次使用时自动创建。

| 表 | 说明 |
|----|------|
| page_index | 所有页面的索引 |
| active_page | 每个 cwd 的活跃页面 |
| config | key-value 全局配置 |

```sql
CREATE TABLE page_index (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    iroll_name TEXT NOT NULL,
    page_id TEXT NOT NULL,
    cwd TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE active_page (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    cwd TEXT NOT NULL UNIQUE,
    iroll_name TEXT NOT NULL,
    page_id TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
```

---

## 7. 时间格式

所有时间字段使用 **ISO 8601 / RFC 3339 Nano** 格式: `2026-06-11T10:00:00.000000000Z`。

---

## 8. 安全约束

- **iroll 名称**: 仅允许 `a-zA-Z0-9._-`，不能包含路径分隔符或 `..`
- **路径安全**: 所有文件读写通过 `safepath` 校验，禁止符号链接逃逸包根目录
- **@sql 无沙箱**: context 中的 `@sql` 直接执行，不做权限隔离。包的制作者拥有完全信任 — 不信任的包不要加载
- **skill 脚本全权限**: skill 的脚本执行时无限制。信任包的制作者
