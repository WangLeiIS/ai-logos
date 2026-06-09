# Loop 表重命名设计

## 1. 概念变更

`heartbeat` 重命名为 `loop`。heartbeat 概念太窄（只是 loop 的一种），loop 是更高层的抽象——heartbeat、breath、周期检查、一次性任务都是 loop。agent 的运行模式通过 loop 来定义。

新增 `name`（短标识索引）和 `describe`（简短描述）字段，与 dna 表风格一致。

## 2. 表结构

```sql
CREATE TABLE loop (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL,
    name TEXT NOT NULL,
    describe TEXT NOT NULL,
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
| type | TEXT | NOT NULL | 任务类型：`once` / `periodic` |
| name | TEXT | NOT NULL | 短标识，如 `self-cognition` |
| describe | TEXT | NOT NULL | 简短描述，如 "自我认知" |
| content | TEXT | NOT NULL | 完整任务指令，agent 读取执行 |
| status | TEXT | NOT NULL | once: `pending` / `done`；periodic: 始终 `active` |
| executed_count | INTEGER | DEFAULT 0 | 执行次数计数器 |
| result | TEXT | DEFAULT '' | 执行结果 |
| weight | REAL | DEFAULT 0.5 | 优先级权重 |
| created_at | TEXT | NOT NULL | 创建时间 |
| updated_at | TEXT | NOT NULL | 更新时间 |

## 3. 默认数据

```sql
INSERT INTO loop (type, name, describe, content, status, executed_count, result, weight, created_at, updated_at) VALUES
    ('once', 'self-cognition', '自我认知', '阅读所有 context 和 dna，了解自己的身份', 'pending', 0, '', 0.9, datetime('now'), datetime('now')),
    ('periodic', 'daily-check', '日常检查', '每次对话开始时，检查 dna 和 memory', 'active', 0, '', 0.8, datetime('now'), datetime('now'));
```

## 4. 实现清单

- [ ] `examples/base-agent/init_schema.sql` — 表名 `heartbeat` → `loop`，新增 `name`、`describe` 字段
- [ ] `examples/base-agent/init_data.sql` — INSERT 改为 `loop` 表，加入 `name`、`describe` 值
- [ ] `docs/rebot-roll.md` — 表名、字段文档、路线图
- [ ] `skills/logos-1/skill.md` — Key Concepts
- [ ] `docs/superpowers/specs/2026-06-08-heartbeat-table-design.md` — 更新或替换
