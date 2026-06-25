# CLI 三段式输出

## 目标

CLI 输出改为结构化三段式，统一成功/失败格式，全部输出到 stdout，方便 AI agent 程序化消费。

## 一、输出格式

每行一个 JSON，固定顺序：

```
Line 1: {"status":"ok"}  或  {"status":"error","code":"...","error":"..."}
Line 2: data（可选，成功时输出；失败时跳过）
Line 3: {"hints":[{"action":"...","cmd":"..."}]}（可选，无 hint 时跳过）
```

全部输出到 **stdout**。`outputFail` 打印 error JSON 后 `os.Exit(1)`，exit code 保持语义。

### 成功示例

```json
{"status":"ok"}
{"page_id":"041c91d0-...","cwd":"C:\\Users\\Meide\\.iroll\\cat\\base\\workspace","alias":"","created_at":"2026-06-25T06:56:08Z"}
{"hints":[{"action":"Set an alias for this page, so you can reference it by name later","cmd":"logos page update-context --page 041c91d0-... --set-alias <name>"},{"action":"Get the full context including DNA, loops and system prompt","cmd":"logos page get-context --page 041c91d0-..."}]}
```

### 失败示例

```json
{"status":"error","code":"invalid_tag","error":"invalid tag: parse error"}
{"hints":[{"action":"List all available iroll packages","cmd":"logos status --list"},{"action":"Build an iroll from an Irollfile","cmd":"logos roll build -f <Irollfile> -t <name>"}]}
```

## 二、新增辅助函数（root.go）

```go
// outputOK prints success three-line output to stdout.
// data and hints are optional (nil/empty = line skipped).
func outputOK(data interface{}, hints []Hint)

// outputFail prints error three-line output to stdout, then os.Exit(1).
// hints is optional.
func outputFail(code, errMsg string, hints []Hint)

// Hint is a structured suggestion for the next command.
type Hint struct {
    Action string `json:"action"`
    Cmd    string `json:"cmd"`
}
```

## 三、PageBrief struct（db/db.go）

```go
type PageBrief struct {
    PageID    string `json:"page_id"`
    Cwd       string `json:"cwd"`
    Alias     string `json:"alias,omitempty"`
    CreatedAt string `json:"created_at"`
}
```

`page new` 成功返回 `PageBrief`（不含 context），引导 agent 通过 `get-context` 获取完整上下文。

## 四、错误码

| 常量 | 值 | 含义 |
|---|---|---|
| `ErrCodeInvalidTag` | `"invalid_tag"` | tag 解析失败 |
| `ErrCodeIrollNotFound` | `"iroll_not_found"` | iroll 包不存在 |
| `ErrCodeNoDefaultPage` | `"no_default_page"` | 没有设置默认 page |
| `ErrCodePageNotFound` | `"page_not_found"` | page 不存在 |
| `ErrCodeDBOpen` | `"db_open_failed"` | 数据库打开失败 |
| `ErrCodeInternal` | `"internal"` | 内部错误 |

使用 string 常量，不用枚举值——人和 agent 都可读。

## 五、page new 改造

### 成功 hints

```json
[
  {"action":"Set an alias for this page, so you can reference it by name later","cmd":"logos page update-context --page <page-id> --set-alias <name>"},
  {"action":"Get the full context including DNA, loops and system prompt","cmd":"logos page get-context --page <page-id>"}
]
```

### 失败 hints

| 错误码 | hints |
|---|---|
| `invalid_tag` | 查看 iroll 列表、构建 iroll |
| `iroll_not_found` | 查看 iroll 列表 |
| 其他 | 无 |

## 六、涉及文件

| 层 | 文件 | 变更 |
|---|---|---|
| cmd | `root.go` | 新增 `outputOK`、`outputFail`、`Hint`、错误码常量 |
| db | `db.go` | 新增 `PageBrief` struct |
| cmd | `page.go` | `pageNewCmd` 改为三段式输出 |

## 七、向后兼容

- `outputJSON` / `outputError` **暂保留**，其他命令未迁移时继续使用
- 本次仅改造 `page new`，后续命令逐个迁移
- `outputError` 改为调用 `outputFail`（内部统一），但保留旧函数签名方便渐进迁移

## 八、非目标

- 不改其他命令（list、switch、delete、get-context 等）
- 不改 `db.Page` struct
- 不引入 CLI 输出框架/库
