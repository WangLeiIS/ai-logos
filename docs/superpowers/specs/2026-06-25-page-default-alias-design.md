# page default + alias + get-context 统一入口

## 目标

解决 page 跨目录访问问题：用户在其他目录也能通过 `--roll`（默认 page）、`--alias`（别名）、`--page`（ID）获取上下文。

## 一、`page default` 命令

```bash
logos page default <page-id>             # 设为所属 iroll 的默认 page
logos page default --roll cat            # 查看 cat 当前默认 page
logos page default --roll cat --clear    # 清除
```

底层存 `system.db` 的 `config` 表：
```
config.key = "default_page:<iroll_name>"  → config.value = "<page_id>"
```

per-iroll，每个 iroll 最多一个默认 page。

## 二、alias 体系

### 数据库新增列

| 表 | 新列 |
|---|---|
| `pages`（init_outer.sql 的 schema + init_inner.sql 模板） | `alias TEXT`（可为 NULL） |
| `page_index`（system.db） | `alias TEXT`（可为 NULL） |

`page_index.alias` 是跨目录查找的核心——`--alias` 直接查 system.db，不需要打开 iroll db。

### 命令

```bash
logos update-context --page <id> --set-alias mycat   # 设置别名
logos update-context --page <id> --set-alias ''      # 清除别名
```

设置时同时更新 iroll 的 `pages.alias` 和 system.db 的 `page_index.alias`。

## 三、`get-context` 统一入口

| 命令 | 查找方式 | 跨目录 |
|---|---|---|
| `get-context` | `GetActive(cwd)`，cwd 默认 `.` | ❌ |
| `get-context .` | 同上 | ❌ |
| `get-context --cwd <path>` | `GetActive(abs(path))` | ✅ |
| `get-context --roll cat` | `config.default_page:cat` → `page_index` | ✅ |
| `get-context --alias mycat` | `page_index WHERE alias = ?` | ✅ |
| `get-context --page <id>` | `page_index WHERE page_id = ?` | ✅ |

优先级：`--page` > `--alias` > `--roll` > `--cwd` / 位置参数

`--page` / `--alias` / `--roll` 不依赖 cwd。

## 四、`page new` 自动设默认

workspace 模式（不带 `.` 和 `--cwd`）：
```
page new cat
  → 建完后自动写 config.default_page:cat = <new_page_id>
```

指定 cwd 模式（`page new cat .` 等）不设默认——用户显式选了目录。

## 五、`page switch` 不变

保持 cwd 级别行为，不涉及全局默认。

## 六、数据库迁移

### init_outer.sql + init_inner.sql

`pages` 表加列：
```sql
alias TEXT
```

### system.db

`page_index` 表加列（migration）：
```sql
ALTER TABLE page_index ADD COLUMN alias TEXT
```

### init_data.sql

模板 page 0 的 alias 为 NULL。

## 七、新增 store 函数

```go
// SetDefaultPage sets the default page for an iroll name.
func SetDefaultPage(name, pageID string) error

// GetDefaultPage returns the default page_id for an iroll name.
func GetDefaultPage(name string) (string, error)

// ClearDefaultPage removes the default page for an iroll name.
func ClearDefaultPage(name string) error

// LookupPageByAlias looks up a page by alias from page_index.
func LookupPageByAlias(alias string) (name, version, pageID, outerDbPath string, error)
```

## 八、涉及文件

| 层 | 文件 | 变更 |
|---|---|---|
| schema | `init_inner.sql`, `init_outer.sql` | `pages` 表加 `alias TEXT` |
| data | `init_data.sql` | 模板 page alias = NULL |
| system | `store/system.go` | migration + 新增函数 |
| db | `db/db.go` | `UpdatePageContext` 支持 alias |
| cmd | `context.go` | `get-context` 加 `--roll`/`--alias` flags，`resolvePage` 重构 |
| cmd | `page.go` | 新增 `pageDefaultCmd`，`pageNewCmd` 设默认 |
