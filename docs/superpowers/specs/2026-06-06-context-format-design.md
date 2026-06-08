# Context 字段标准化格式设计

## 背景

`pages.context` 是 logos 系统的核心字段，存储 agent 的行为指令。当前只支持纯字符串值，需要扩展为支持三种数据源：纯字符串、文件引用、SQL 查询。

## 存储格式

`pages.context` 存储 JSON 对象，key 自由定义，value 支持三种类型：

| 类型 | 格式 | 示例 |
|------|------|------|
| 纯字符串 | 直接写值 | `"你是一个助手"` |
| 文件引用 | `{"@file": "路径"}` | `{"@file": "Resources/greeting.txt"}` |
| SQL 查询 | `{"@sql": "SQL"}` | `{"@sql": "SELECT value FROM metadata WHERE key = 'description'"}` |

完整示例：

```json
{
  "system_prompt": "你是一个助手",
  "greeting": {"@file": "Resources/greeting.txt"},
  "description": {"@sql": "SELECT value FROM metadata WHERE key = 'description'"}
}
```

**key 完全自由**，用户可以定义任意 key。

**文件路径** 相对于 iroll 包根目录（`~/.iroll/<name>/`）。

**SQL 查询** 执行在当前 iroll 的 `ai_roll.db` 上。

## 行为规则

### 写入（update-context）

原样存入，不做解析。用户传入什么就存什么。

### 读取（get-context / page new）

遍历 JSON 的每个 value，按类型解析：

- **纯字符串**：原样返回
- **`{"@file": "..."}`**：读取文件内容返回字符串。文件不存在则返回 `"[file not found: <path>]"`
- **`{"@sql": "..."}`**：执行查询。单行单列返回字符串值；多行结果返回字符串数组；查询失败返回 `"[sql error: <message>]"`

### 解析上下文

`get-context` 通过 `--cwd` 找到活跃 page → 得到 iroll_name → 知道 iroll 包路径和数据库路径。不需要额外参数。

## 改动范围

1. **`db/db.go`** — 新增 `ResolveContext` 函数，遍历 JSON 解析 @file 和 @sql
2. **`cmd/page.go`** — `page new` 返回时调用解析
3. **`cmd/context.go`** — `get-context` 返回时调用解析
4. **`examples/base-agent/init_data.sql`** — 模板页 context 改为新格式
5. **`skills/logos-1/skill.md`** — 更新说明
