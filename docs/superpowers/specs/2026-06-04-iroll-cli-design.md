# iroll CLI 管理工具设计

## 概述

一个 Go 语言编写的 CLI 工具（Cobra），用于管理 .iroll 包。面向 agent 使用，所有输出为 JSON。.iroll 文件加载后解压到固定工作目录，后续操作直接读写 SQLite 数据库，不需要重新打包。

## 工作目录

- 固定路径：`~/.iroll/`
- 每个 iroll 加载后解压为子目录，子目录名即 iroll 名称，不可重复
- 结构：
  ```
  ~/.iroll/
  ├── my-agent/
  │   ├── Resources/
  │   └── ai_roll.db
  └── another-agent/
      ├── Resources/
      └── ai_roll.db
  ```

## CLI 命令

### `iroll load <file-path>`

将 .iroll 文件解压到 `~/.iroll/<name>/`。

- 从 ZIP 文件中提取 `ai_roll.db` 的 metadata 表读取 name 作为目录名
- 如果同名已存在，报错退出
- 输出：`{"name": "my-agent", "path": "~/.iroll/my-agent"}`

### `iroll list`

列出所有已加载的 iroll。

- 输出：`[{"name": "my-agent"}, {"name": "another-agent"}]`

### `iroll session list <name> --cwd <dir>`

列出指定 cwd 下的所有 session。

- `--cwd` 必填
- 查询 context 表中 cwd 匹配的记录
- 输出：`[{"id": 1, "session_id": "sess-xxx", "cwd": "/project", "content": "...", "created_at": "...", "updated_at": "..."}]`

### `iroll session init <name> --cwd <dir>`

创建新会话，插入一条 context 记录。

- `--cwd` 必填
- 自动生成 session_id（UUID）
- content 初始为空字符串
- 输出：`{"id": 1, "session_id": "sess-xxx", "cwd": "/project", "content": "", "created_at": "...", "updated_at": "..."}`

### `iroll get-context <name> --session <session-id>`

按 session_id 获取 context 记录。

- `--session` 必填
- 输出：`{"id": 1, "session_id": "sess-xxx", "cwd": "/project", "content": "...", "created_at": "...", "updated_at": "..."}`

### `iroll update-context <name> --session <session-id> --content <new-content>`

更新指定 session 的 context 内容。

- `--session` 和 `--content` 必填
- 同时更新 updated_at
- 输出：`{"id": 1, "session_id": "sess-xxx", "content": "new content", "updated_at": "..."}`

### `iroll add-memory <name> --content <content> [--importance <0.0-1.0>]`

新增一条记忆。

- `--content` 必填
- `--importance` 可选，默认 0.5
- 输出：`{"id": 1, "content": "...", "importance": 0.8, "created_at": "..."}`

## 数据库表结构

操作对象为 .iroll 包内的 `ai_roll.db`，表结构：

### metadata 表

| 字段 | 类型 | 约束 |
|------|------|------|
| key | TEXT | NOT NULL |
| value | TEXT | NOT NULL |
| remark | TEXT | |
| created_at | TEXT | NOT NULL |
| updated_at | TEXT | NOT NULL |

### memory 表

| 字段 | 类型 | 约束 |
|------|------|------|
| id | INTEGER | PRIMARY KEY AUTOINCREMENT |
| content | TEXT | NOT NULL |
| created_at | TEXT | NOT NULL |
| importance | REAL | DEFAULT 0.5 |

### context 表

| 字段 | 类型 | 约束 |
|------|------|------|
| id | INTEGER | PRIMARY KEY AUTOINCREMENT |
| session_id | TEXT | NOT NULL |
| cwd | TEXT | |
| content | TEXT | NOT NULL |
| created_at | TEXT | NOT NULL |
| updated_at | TEXT | NOT NULL |

## Go 项目结构

```
iroll/
├── main.go              # 入口
├── cmd/
│   ├── root.go          # 根命令 + 全局 --name flag
│   ├── load.go          # load 子命令
│   ├── list.go          # list 子命令
│   ├── session.go       # session list / session init 子命令
│   ├── context.go       # get-context / update-context 子命令
│   └── memory.go        # add-memory 子命令
├── db/
│   └── db.go            # SQLite CRUD 操作
├── store/
│   └── store.go         # 工作目录管理（解压、路径解析、名称读取）
├── go.mod
└── go.sum
```

## 依赖

- `github.com/spf13/cobra` — CLI 框架
- `github.com/mattn/go-sqlite3` — SQLite 驱动（CGO）
- `github.com/google/uuid` — session_id 生成

## 约束

- 所有输出 JSON，成功时输出数据，失败时输出 `{"error": "message"}` 并 exit 1
- 时间格式 ISO 8601 UTC
- 不实现 .iroll 创建功能
