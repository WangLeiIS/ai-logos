# Memory 系统重构

> **日期:** 2026-06-10
> **参考:** [rebot-roll.md](../../rebot-roll.md) 第 3.2 节、第 8 节

## 目标

重新设计 memory 表结构（参考 dna 的 Q&A 模式），去掉手动 add-memory CLI，补齐查询 API。

## 表结构

### 新 memory 表

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

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PRIMARY KEY AUTOINCREMENT | 主键 |
| page_id | TEXT | NOT NULL | 所属页面 |
| name | TEXT | NOT NULL | 索引名，agent 自行命名，如 `user-prefers-python-312` |
| question | TEXT | NOT NULL | 提出什么问题能触发这条记忆 |
| content | TEXT | NOT NULL | 记忆的具体内容（sleep 整理后可能被精简） |
| importance | REAL | NOT NULL DEFAULT 0.5 | 重要度 0.0-1.0 |
| sleep_count | INTEGER | NOT NULL DEFAULT 0 | sleep 循环已处理次数，每次整理后 +1 |
| created_at | TEXT | NOT NULL | 创建时间 |
| updated_at | TEXT | NOT NULL | 更新时间 |

### 和 dna 表的对应

| dna | memory | 说明 |
|-----|--------|------|
| name | name | 唯一标识/索引 |
| type | — | dna 有维度分类，memory 不需要 |
| question | question | 提出什么问题能触发这条记录 |
| answer | content | 记录的具体内容 |
| weight | importance | 重要程度 |
| — | sleep_count | memory 特有：sleep 处理计数 |

### 和旧 memory 表的差异

| 变更 | 说明 |
|------|------|
| + page_id | 按页面隔离记忆 |
| + name | 索引名，agent 自行命名 |
| + question | 问题列，query 时匹配此列 |
| + updated_at | sleep 整理时更新 |
| + sleep_count | sleep 整理次数，新建为 0 |
| - (手动 add-memory) | 去掉，memory 只由 context 压缩自动插入 |
| content | 保持不变，但语义从"模糊内容块"变为"问题的回答" |

## DB 层 API

```go
type Memory struct {
    ID         int64   `json:"id"`
    PageID     string  `json:"page_id"`
    Name       string  `json:"name"`
    Question   string  `json:"question"`
    Content    string  `json:"content"`
    Importance float64 `json:"importance"`
    SleepCount int     `json:"sleep_count"`
    CreatedAt  string  `json:"created_at"`
    UpdatedAt  string  `json:"updated_at"`
}

type MemorySummary struct {
    Name        string `json:"name"`
    Question    string `json:"question"`
    ContentLen  int    `json:"content_len"`
    SleepCount  int    `json:"sleep_count"`
}

type QueryMemoryParams struct {
    Keyword       string // LIKE 匹配 name 或 question
    MinImportance float64 // 0 表示不过滤
    Since         string // ISO timestamp
    Before        string // ISO timestamp
    Limit         int
    Offset        int
}

// InsertMemory 由 context 压缩时调用，agent 自行提供 name 和 question
func InsertMemory(db *sql.DB, pageID, name, question, content string, importance float64) (*Memory, error)

// QueryMemory 按条件查询，返回完整记录，按 importance DESC, created_at DESC 排序
func QueryMemory(db *sql.DB, pageID string, params QueryMemoryParams) ([]Memory, error)

// IncrementSleepCount sleep 整理后调用，计数 +1 并更新 updated_at
func IncrementSleepCount(db *sql.DB, memoryID int64) error

// UpdateMemoryContent sleep 整理后替换精简版内容
func UpdateMemoryContent(db *sql.DB, memoryID int64, content string, importance float64) error
```

InsertMemory 时 `sleep_count` 默认 0。每次 sleep 整理完成后调用 `IncrementSleepCount` 加 1。

## CLI

### 删除

```
logos page add-memory [name] --content <text> [--importance 0.5] [--cwd .]
```

### 新增

```
logos page query-memory [name]
    [--keyword <text>]
    [--min-importance <0.0-1.0>]
    [--since <ts>] [--before <ts>]
    [--limit <n>]
    [--full]
    [--cwd .]
```

| 参数 | 说明 |
|------|------|
| `name` | 可选位置参数，精确匹配 name（与 `--keyword` 互斥） |
| `--keyword` | LIKE 搜索 name 和 question 列 |
| `--min-importance` | 最低重要度过滤 |
| `--since` | ISO 时间，只返回此时间之后的记忆 |
| `--before` | ISO 时间，只返回此时间之前的记忆 |
| `--limit` | 返回条数，默认 20，最大 100 |
| `--full` | 返回完整记录（含 content），默认只输出摘要 |
| `--cwd` | 工作目录，默认 `.` |

**默认输出（摘要）：** `(name, question, content_len, sleep_count)`，agent 先浏览索引找到目标
**`--full` 输出（完整）：** `(id, page_id, name, question, content, importance, sleep_count, created_at, updated_at)`

### 查询示例

```bash
# 索引浏览：看当前页面有哪些记忆（默认摘要模式）
logos page query-memory

# 按关键词搜索（摘要）
logos page query-memory --keyword "Python"

# 只看重要的，返回完整内容
logos page query-memory --min-importance 0.7 --full

# 最近一周的（摘要）
logos page query-memory --since "2026-06-03T00:00:00Z"

# 精确查某条，必须拿到完整内容
logos page query-memory user-prefers-python-312 --full
```

## 涉及文件

| 文件 | 操作 |
|------|------|
| `iroll/db/db.go` | 修改 Memory struct、新增 MemorySummary、QueryMemoryParams、InsertMemory、QueryMemory、IncrementSleepCount、UpdateMemoryContent |
| `iroll/cmd/memory.go` | 重写：删除 add-memory，新增 query-memory |
| `examples/base-agent/init_schema.sql` | 更新 CREATE TABLE memory |
| `docs/rebot-roll.md` | 已更新（3.2 节、5.3 节、7 节、8.3 节） |
| `docs/todo.md` | 已更新 |

## sleep 场景

```
sleep 触发
  → GetActiveMainLoopRun(pageID) 检查是否有活跃的 sleep 主运行
  → 没有则 StartLoopRun(pageID, "sleep", nil, plan)
  → 遍历 memory WHERE sleep_count = 0（优先处理未整理的新记忆）
  → 对每条 memory：
      1. 提炼：提取核心信息，精简冗余
      2. UpdateMemoryContent(id, 精简后的content, 调整后的importance)
      3. IncrementSleepCount(id)  → sleep_count + 1
      4. 次要细节 → 移入 forget 表（后续实现）
  → CompleteLoopRun(pageID, runID, result)
```
