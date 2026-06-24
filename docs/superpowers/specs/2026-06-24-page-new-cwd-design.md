# page new cwd 逻辑改进

## 目标

`logos page new` 的 cwd 参数更灵活：支持位置参数、默认 workspace 目录。

## 三种 cwd 指定方式

| 优先级 | 方式 | cwd 值 |
|---|---|---|
| 1（最高） | `--cwd <path>` 显式指定 | 绝对路径 |
| 2 | 第二个位置参数 | 绝对路径 |
| 3（默认） | 都不给 | `~/.iroll/<name>/<version>/workspace/` |

## 命令示例

```bash
logos page new my-agent                    # cwd → ~/.iroll/my-agent/latest/workspace/
logos page new my-agent .                  # cwd → 当前目录绝对路径
logos page new my-agent ./cat-data         # cwd → /abs/path/to/cat-data
logos page new my-agent /some/path         # cwd → /some/path
logos page new my-agent --cwd /tmp/test    # cwd → /tmp/test（覆盖所有）
logos page new my-agent . --cwd /tmp/test  # --cwd 覆盖位置参数
```

## 规则

- `<iroll-name>` 始终是第一个参数，必传
- 存储到数据库的 cwd 始终是绝对路径（`filepath.Abs()`）
- 默认 workspace 目录不存在时自动创建（`os.MkdirAll`）
- `Args: ExactArgs(1)` → `RangeArgs(1, 2)`
- `--cwd` flag 默认值从 `"."` 改为空字符串 `""`
- 判断逻辑：`--cwd` 优先 → 第二位置参数 → workspace 默认

## 涉及文件

- `iroll/cmd/page.go` — `pageNewCmd` 参数和 cwd 判断逻辑

## 不影响

- `db.InsertPage` / `store.IndexPage` / `store.GetActive` — 接口不变
- `page list` / `page current` / `page switch` / `page delete` — 不变
- `AutoStartLoopSeeds` — 不变
