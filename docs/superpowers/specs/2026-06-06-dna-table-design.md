# DNA 表设计

## 1. 概念定位

dna 表定义 agent 做决策时的底层机制，不直接描述表面性格标签，而是通过具体情境中的选择来体现 agent 的特质。

四个决策维度（type）：

| type | 含义 | 示例情境 |
|------|------|----------|
| 认知观 | 如何看待信息和真相，相信什么证据 | "用户说你错了，但你确定自己正确 → 坚持？退让？" |
| 伦理观 | 如何判断对错，结果优先还是规则优先 | "说实话会伤害感情 → 委婉？坦诚？" |
| 审美观 | 什么算"好"的解决方案 | "3 行能跑 vs 50 行覆盖所有边界 → 选哪个？" |
| 本体观 | 什么在这个 agent 的世界里是真实和重要的 | "代码只是工具？还是代码本身有尊严？" |

## 2. 表结构

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

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PRIMARY KEY AUTOINCREMENT | 主键 |
| name | TEXT | NOT NULL | 唯一标识，如 `handle-correction`、`truth-vs-feelings` |
| type | TEXT | NOT NULL | 认知观 / 伦理观 / 审美观 / 本体观 |
| question | TEXT | NOT NULL | 决策困境 |
| answer | TEXT | NOT NULL | 这个 agent 的选择 |
| weight | REAL | DEFAULT 0.5 | 权重，越高越核心 |
| created_at | TEXT | NOT NULL | 创建时间 |
| updated_at | TEXT | NOT NULL | 更新时间 |

## 3. 使用方式

### 3.1 Context 加载（轻量）

模板页面 context 通过 `@sql` 加载 dna，只查 `type`、`weight`、`question`，answer 不加载：

```json
{
  "system_prompt": "你是一个AI助手",
  "greeting": {"@file": "Resources/greeting.txt"},
  "description": {"@sql": "SELECT value FROM metadata WHERE key = 'description'"},
  "dna": {"@sql": "SELECT type, weight, question FROM dna ORDER BY weight DESC"}
}
```

agent 获取 context 后看到自己面临的情境和选择原则，answer 不进入上下文以节省 token。

### 3.2 按需查询（完整）

```bash
logos page query-dna <name> [--type <type>] [--cwd .]
```

- `name`（必选）：定位参数，通过 `LIKE %name%` 模糊匹配
- `--type`（可选）：按类型过滤（认知观 / 伦理观 / 审美观 / 本体观）

返回完整字段（含 answer）：

```json
{
  "id": 1,
  "name": "handle-correction",
  "type": "认知观",
  "question": "用户说你错了，但你确定自己正确",
  "answer": "坚持己见，但给出理由而非争论",
  "weight": 0.9,
  "created_at": "2026-06-06T...",
  "updated_at": "2026-06-06T..."
}
```

## 4. 示例数据

```sql
INSERT INTO dna (name, type, question, answer, weight, created_at, updated_at) VALUES
    ('handle-correction', '认知观', '用户说你错了，但你确定自己正确', '坚持己见，给出理由而非争论', 0.9, datetime('now'), datetime('now')),
    ('truth-vs-feelings', '伦理观', '说实话会伤害用户感情', '坦诚告知，但用建设性的方式表达', 0.8, datetime('now'), datetime('now')),
    ('minimal-vs-complete', '审美观', '3行能跑但不够健壮 vs 50行覆盖所有边界', '先交付简洁方案，告知边界条件', 0.7, datetime('now'), datetime('now'));
```

## 5. 实现清单

- [ ] `examples/base-agent/init_schema.sql` — 添加 dna 表
- [ ] `examples/base-agent/init_data.sql` — 添加示例 dna 数据
- [ ] `examples/base-agent/init_data.sql` — 模板 page context 添加 dna 查询
- [ ] `iroll/cmd/query_dna.go` — 新增 `logos page query-dna` 命令
- [ ] `iroll/db/db.go` — 添加 QueryDna 函数
- [ ] `docs/rebot-roll.md` — 更新文档（dna 表、query-dna 命令）
- [ ] `skills/logos-1/skill.md` — 更新命令参考表
