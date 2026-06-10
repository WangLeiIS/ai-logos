# Heartbeat 表设计

> **已废弃：** 本设计已被 [Loop 表重命名设计](2026-06-08-loop-table-rename.md) 替代。当前实现和使用文档统一使用 `loop`；本文仅保留为历史设计记录。

## 1. 概念定位

heartbeat 是 agent 的待办任务表。记录 agent 需要执行的事项，分为两种类型：

| 类型 | 说明 | 生命周期 |
|------|------|----------|
| once（一次性） | 执行一次即完成 | pending → done |
| periodic（周期） | 可反复执行，始终 active | active + executed_count 递增 |

heartbeat 不是调度器，而是一种记录方式。agent 在对话中主动检查待办并执行。

## 2. 表结构

```sql
CREATE TABLE heartbeat (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL,
    content TEXT NOT NULL,
    status TEXT NOT NULL,
    executed_count INTEGER DEFAULT 0,
    result TEXT DEFAULT '',
    weight REAL DEFAULT 0.5,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
```

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PRIMARY KEY AUTOINCREMENT | 主键 |
| type | TEXT | NOT NULL | 任务类型：`once` 或 `periodic` |
| content | TEXT | NOT NULL | 任务描述，纯文本，agent 自由解读 |
| status | TEXT | NOT NULL | once: `pending` / `done`；periodic: 始终 `active` |
| executed_count | INTEGER | DEFAULT 0 | 执行次数计数器，periodic 专用 |
| result | TEXT | DEFAULT '' | 执行结果，periodic 每次覆盖为最新结果 |
| weight | REAL | DEFAULT 0.5 | 优先级权重，越高越优先处理 |
| created_at | TEXT | NOT NULL | 创建时间 |
| updated_at | TEXT | NOT NULL | 更新时间 |

## 3. 初始化任务

创建 heartbeat 表时插入两条默认任务：

```sql
INSERT INTO heartbeat (type, content, status, executed_count, result, weight, created_at, updated_at) VALUES
    ('once', '阅读所有 context 和 dna，了解自己的身份', 'pending', 0, '', 0.9, datetime('now'), datetime('now')),
    ('periodic', '每次对话开始时，检查 dna 和 memory', 'active', 0, '', 0.8, datetime('now'), datetime('now'));
```

## 4. 使用方式

### 4.1 查看待办

agent 在对话开始时查询待执行任务：

```sql
SELECT id, type, content, status, executed_count, result, weight
FROM heartbeat
WHERE status IN ('pending', 'active')
ORDER BY weight DESC
```

### 4.2 完成一次性任务

执行完毕后更新状态和结果：

```sql
UPDATE heartbeat SET status = 'done', result = '已完成自我认知，我是 test-agent', updated_at = datetime('now')
WHERE id = ?
```

### 4.3 执行周期任务

每次执行后递增计数并更新结果：

```sql
UPDATE heartbeat SET executed_count = executed_count + 1, result = '已检查 3 条 dna 和 5 条 memory', updated_at = datetime('now')
WHERE id = ?
```

## 5. 实现清单

- [ ] `examples/base-agent/init_schema.sql` — 添加 heartbeat 表
- [ ] `examples/base-agent/init_data.sql` — 添加默认任务
- [ ] `docs/rebot-roll.md` — 更新文档（heartbeat 表结构、去掉"待定义"）
- [ ] `skills/logos-1/skill.md` — 更新 Key Concepts
