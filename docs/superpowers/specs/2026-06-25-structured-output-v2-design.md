# CLI 三段式输出 整体改造 V2 — 设计规格

> 继 V1（page new / get-context / update-context）之后，完成 page 组 + 高频命令的全量三段式改造。

## 输出格式（不变）

```
{"status":"ok"}
<data JSON line, omitted if data is nil>
{"hints":[{"action":"...","cmd":"..."}], omitted if hints empty}
```

错误路径：
```
{"status":"error","code":"<code>","error":"<message>"}
{"hints":[...], omitted if hints empty}
```

所有输出到 stdout，`os.Exit(1)` 区分成功/失败。

## 改造范围

### 本次改造的命令

| 文件 | 命令 | 改动类型 |
|---|---|---|
| `root.go` | `checkedIrollPath`, `checkedInnerPath`, `openOuterFromActive` | 辅助函数：`outputError` → `outputFail` |
| `page.go` | `page list` (两条路径), `page switch`, `page delete`, `page default` (set/show/clear/error) | `outputJSON`/`outputError` → `outputOK`/`outputFail` + hints |
| `page.go` | `page current` | **删除整个命令** |
| `query_dna.go` | `query-dna` | `outputJSON`/`outputError` → `outputOK`/`outputFail` + hints |
| `loop_run.go` | `loop-run start/abort/complete/reflect/list/get` | `outputJSON`/`outputError` → `outputOK`/`outputFail` + hints |
| `loop_seed.go` | `loop-seed list/start/abort/complete/reflect/reset/remove` | `outputJSON`/`outputError` → `outputOK`/`outputFail` + hints |
| `memory.go` | `memory query/summary` | `outputJSON`/`outputError` → `outputOK`/`outputFail` + hints |
| `evolving.go` | `evolving run` | `outputJSON`/`outputError` → `outputOK`/`outputFail` + hints |

### 不改造的命令（暂缓）

`build`, `list`, `inspect`, `load`, `rm`, `save`, `status`, `version`, `hub_pull`, `hub_push`, `hub_login`, `hub_search`, `history`, `book`, `skill`

这些命令继续使用 `outputJSON`/`outputError`（两个旧函数保留在 root.go 不动）。

## 数据原则

### Page 命令 === PageBrief

所有返回值含 page 数据的命令统一使用 `PageBrief`（`page_id`, `cwd`, `alias,omitempty`, `created_at`），不返回 `context`、`id`、`updated_at`。

`page list` 的 store 路径（`-a` 无参）返回 `[]map[string]interface{}`，已不含 context，不需要改结构。

### Loop/Memory/Evolving 命令

返回现有 struct，这些 struct 本身不含需要"延迟加载"的重字段。

## 新增错误码

```go
ErrCodeNoActivePage = "no_active_page"
```

完整错误码清单（7 个）：

| 常量 | 值 | 用途 |
|---|---|---|
| `ErrCodeInvalidTag` | `invalid_tag` | 标签格式错误 |
| `ErrCodeIrollNotFound` | `iroll_not_found` | iroll 包不存在 |
| `ErrCodeNoDefaultPage` | `no_default_page` | 没有默认 page |
| `ErrCodeNoActivePage` | `no_active_page` | 当前 cwd 没有 active page |
| `ErrCodePageNotFound` | `page_not_found` | page 不存在 |
| `ErrCodeDBOpen` | `db_open_failed` | 数据库打开失败 |
| `ErrCodeInternal` | `internal` | 内部错误（兜底） |

## root.go 辅助函数改造

3 个函数，各 1 处 `outputError` → `outputFail(ErrCodeInternal, msg, nil)`。

例外：`openOuterFromActive` 在无 active page 时用 `ErrCodeNoActivePage` 并带 hints（建议 `page new` 或 `page list -a`）。

## 删除 page current

`page current` 的所有功能已被 `get-context`（无参数 cwd 回退）和 `page list --cwd .` 覆盖，且返回的是未解析 context（与"去掉 context"的核心理念矛盾）。

改动：
- 删除 `pageCurrentCmd` 定义（`page.go:23-43`）
- 删除 `pageCurrentCwd` 变量
- 删除 `init()` 中的 `pageCurrentCmd` 注册和 flag 绑定

## Hints 规格

### 各命令 success hints

| 命令 | Hints |
|---|---|
| `page list` | 若结果非空：`get-context --page <first.page_id>`；始终：`page new <iroll_name>` |
| `page switch` | `get-context --page <page_id>` |
| `page delete` | `page list -a`；`page new <iroll_name>` |
| `page default --set` | `get-context --page <page_id>` |
| `page default --show` | `page default <page_id> --set`；`page default --roll <name> --clear` |
| `page default --clear` | `page default <page_id>` |
| `query-dna` | 非空时取第一条：`page update-context --page <page_id> --content <json>` |
| `loop-run start` | `loop-run get --id <run_id>`；`loop-run list` |
| `loop-run abort/complete` | `loop-run start --seed <seed>`；`loop-run list` |
| `loop-run reflect` | `loop-run get --id <run_id>` |
| `loop-run list/get` | `loop-run start --seed <seed>` |
| `loop-seed list` | `loop-seed start --name <first.name>`（若非空） |
| `loop-seed start/abort/complete/reflect/reset` | `loop-seed list`；`loop-seed start --name <name>` |
| `loop-seed remove` | `loop-seed list` |
| `memory query` | `memory summary` |
| `memory summary` | `memory query --keyword <keyword>` |
| `evolving run` | `page update-context --page <page_id> --content <json>` |

### Hint 生成策略

- **每个命令内联** hints 构造，不复用超过 2 次不提取
- `getContextHints(p)` 保留（已在 `get-context` 和 `update-context` 中复用）

## 错误处理规格

每个命令的每个 `outputError` 调用点改为 `outputFail(code, msg, hints)`：

- **参数/输入错误** → `ErrCodeInvalidTag` 或 `ErrCodePageNotFound`，带恢复 hints
- **数据库/文件 IO 错误** → `ErrCodeDBOpen` 或 `ErrCodeInternal`，hints 为 nil
- **无 active page** → `ErrCodeNoActivePage`，hints 建议 `page new` 或 `page list -a`
- **命令缺参** → `ErrCodeInternal`，hints 为 nil（或提示 usage）

## 不变项

- `outputJSON` / `outputError` 保留在 root.go（仍有 100+ 处调用，其他文件暂不改）
- `outputOK` / `outputFail` 签名不变
- `PageBrief` / `Hint` / 7 个错误码 不变（仅新增 `ErrCodeNoActivePage`）
- 测试全部通过（若有测试断言旧格式则一并更新）

## 与 V1 的关系

V1 spec (`2026-06-25-structured-output-design.md`) 定义了核心基础设施，本 V2 spec 在其上扩展。V1 已完成的命令（page new / get-context / update-context）无需回改，以下例外：

- `resolvePageContext` 的 `openOuterFromActive` 路径：当 cwd 无 active page 时，错误码从 `ErrCodeInternal` 改为 `ErrCodeNoActivePage`，并带 hints。这属于 bug fix，不改变接口。
