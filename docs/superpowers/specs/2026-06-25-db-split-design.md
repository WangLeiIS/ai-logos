# DB 拆分架构：0.1.2 → 0.2.0

## 目标

将单一 `ai_roll.db` 拆分为两个 SQLite 数据库：`roll-inner.db`（内在/只读）和 `roll_outer.db`（外在/读写），实现内在表与工作区数据隔离。

## 架构

### roll-inner.db（内在/系统表）

位于 `~/.iroll/<name>/<version>/roll-inner.db`，构建时写入，运行时不修改。

包含表（全部数据）：
- `metadata` — agent 元数据
- `dna` — 决策基因
- `loop` — 行为种子模板
- `book` — Book Bundle 注册
- `skill` — 技能注册
- `history` — 构建历史
- `pages` — 仅 `page_id = '0'`（模板 context）
- `memory` — 仅 `page_id = '0'`（模板 memory）

### roll-outer.db（外在/工作区表）

运行时读写。多个 page 在同一 (iroll, cwd) 下**共用**一个 outer.db。

包含表（全部行）：
- `pages` — 非模板 pages（page_id ≠ '0'）
- `memory` — 非模板 memories（page_id ≠ '0'）
- `loop_runs` — loop 运行实例

### outer.db 路径规则

| 场景 | 路径 |
|---|---|
| 默认 workspace | `~/.iroll/<name>/<version>/workspace/.<name>.outer.db` |
| 自定义 cwd | `<cwd>/.iroll/<name>.db`（自动创建 `.iroll/` 子目录） |

## SQLite ATTACH 模式

打开数据库时：打开 `roll_outer.db` 作为主库，ATTACH `roll_inner.db` AS `inner`。

- 外层表（pages, memory, loop_runs）不加前缀，直接查本地
- 内层表（metadata, dna, loop, book, skill, history）加 `inner.` 前缀
- 模板数据（page_id='0'）走 `inner.pages` / `inner.memory`

效果：只传一个 `*sql.DB`，改动面最小。

## system.db 扩展

`page_index` 和 `active_page` 各加一列：
- `outer_db_path TEXT NOT NULL` — outer.db 的绝对路径

page new 时写入，后续所有读 page 的命令从此读取。

## Irollfile 指令扩展

新增 `MIGRATE OUTER` 指令：

```Irollfile
MIGRATE init_inner.sql      # → inner.db（默认）
MIGRATE init_data.sql       # → inner.db（seed data）
MIGRATE OUTER init_outer.sql # → outer.db
COPY greeting.txt Resources/greeting.txt
COPY books Resources/books
```

解析规则：
- `MIGRATE <file>` → `InstMigrate`（inner）
- `MIGRATE OUTER <file>` → `InstMigrateOuter`（新增指令类型）

## 构建产物

`.iroll` ZIP 包内：
```
ai_roll.db → roll-inner.db（改名）
roll-outer.db（新增，schema + MIGRATE OUTER 的数据）
Resources/
layer.json
```

## page new 流程

```
1. 解析 iroll name + version
2. 确定 cwd（--cwd > 位置参数 > workspace 默认）
3. 确定 outer.db 路径（路径规则见上）
4. 如果 outer.db 不存在 → 从 iroll 包复制 roll-outer.db 模板
5. OpenOuter(outerPath, innerPath) → ATTACH inner
6. InsertPage → 写入 outer 的 pages 表
7. AutoStartLoopSeeds → 读 inner.loop，写 loop_runs
8. IndexPage → system.db，含 outer_db_path
9. ResolveContext → 读 inner.pages('0'), inner.loop, inner.dna, inner.memory('0')
                        读 loop_runs (outer, 不加前缀)
```

## 代码层面变更概要

### schema 拆分
- `init_schema.sql` → `init_inner.sql` + `init_outer.sql`
- `init_data.sql` 全部数据进 inner

### builder/
- 构建两个 db：`roll-inner.db` + `roll-outer.db`
- 新增 `InstMigrateOuter` 处理
- Irollfile parser 支持 `MIGRATE OUTER`
- ZIP 产物包含两个 db

### db/
- 新增 `OpenOuter(outerPath, innerPath string) (*sql.DB, error)` — 打开 outer，ATTACH inner
- 所有内层表 SQL 加 `inner.` 前缀（metadata, dna, loop, book, skill, history, pages/memory where page_id='0'）
- `InsertPage` / `DeletePage` / `ResolveContext` / `BuildLoopContext` 等适配

### store/
- `DbPath()` → `InnerDbPath()`
- system.db schema 升级：`page_index` + `active_page` 加 `outer_db_path`
- `IndexPage` 签名变更：加 `outerDbPath` 参数
- system.db migration 逻辑

### cmd/
- `checkedDbPath()` → 改为获取 inner + outer 路径
- 所有命令适配新的 db 打开方式
- `pageNewCmd` 增加 outer.db 路径确定逻辑

## 版本策略

1. 当前 main → tag `0.1.2`
2. 实现 DB 拆分 → 版本升 `0.2.0`
