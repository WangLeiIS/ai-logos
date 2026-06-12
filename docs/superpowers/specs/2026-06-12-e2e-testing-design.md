# Logos 端到端测试策略设计

## 目标

为 Logos CLI 建立系统化的端到端测试体系，保障回归安全、发现现有 bug、为未来重构提供安全网。

**范围**：只测 `iroll/` 下的 Go 后端，不包括 `irollhub-web` 前端。

## 现状

- 143 个顶层测试函数，覆盖 7 个包
- DB 层测试扎实（55 个），loop/book 有 CLI 层测试
- **24 个 cmd 源文件中 19 个完全没有测试**，约 25 个 CLI 命令零覆盖
- 1 个已知失败测试（Windows 跨盘符 `filepath.Rel`）
- 6 个 symlink 测试在 Windows 上跳过

## 策略：端到端场景驱动

从用户真实使用流程出发，5 个核心场景覆盖完整生命周期。每个场景直接调用 `db`/`store`/`builder`/`skill`/`book` 等包的公开函数，使用临时 HOME + 真实 SQLite。

不使用 `os/exec` 运行二进制 — 速度快、断言精确、不需要编译。

## 目录结构

```
iroll/e2e/
├── testenv/
│   └── setup.go                        # 共享环境搭建
├── scenario_lifecycle_test.go          # 场景1：构建→加载→查询→删除
├── scenario_page_test.go               # 场景2：页面管理全流程
├── scenario_integration_test.go        # 场景3：DNA+记忆+Loop+Skill 组合
├── scenario_hub_test.go                # 场景4：Hub 集成
└── scenario_edge_test.go               # 场景5：错误与边界
```

## testenv 基础设施

`iroll/e2e/testenv/` 提供共享的环境搭建能力：

```go
type Env struct {
    Home    string       // 临时 HOME 目录
    Store   string       // ~/.iroll/ 路径
}

func New(t *testing.T) *Env
func (e *Env) Build(tagName string) (*builder.BuildResult, error)
func (e *Env) DB(name string) (*sql.DB, error)
func (e *Env) CreatePage(name, pageID string) error
func (e *Env) Cleanup()
```

每个场景测试只需 `env := testenv.New(t)` + `env.Build("test-agent")` 就能获得完整可用的 iroll 包。

## 场景 1：包生命周期（scenario_lifecycle_test.go）

覆盖 `builder`、`store`、`db`(book/skill/history) 的完整生命周期。

| 测试 | 验证 |
|------|------|
| `TestBuildCreatesValidIroll` | 构建后 DB 存在、schema 正确、book/skill 已注册、layer.json 存在 |
| `TestBuildFromInheritsBaseLayer` | FROM 构建继承基座的表数据、资源文件、book 记录 |
| `TestBuildRejectsInvalidTag` | 不合法名称（`..`、`/`、空）被拒绝 |
| `TestListShowsBuiltIroll` | 构建后 `store.List` 能列出 |
| `TestHistoryRecordsBuild` | 构建后 history 表有记录，parent 正确 |
| `TestSaveAndLoadRoundTrip` | 构建 → save 成 ZIP → load 到新名称 → DB 和资源完整 |

## 场景 2：页面管理全流程（scenario_page_test.go）

覆盖 `store`（active_page/page_index）、`db`（pages/context/memory）的页面 CRUD 和 context 解析。

| 测试 | 验证 |
|------|------|
| `TestPageNewInheritsTemplate` | 新页面继承 page_id='0' 的 context |
| `TestPageCurrentResolvesActive` | `store.GetActive` 返回正确的活跃页面 |
| `TestPageListShowsAllPages` | 列出所有页面，含模板页 |
| `TestPageSwitchChangesActive` | 切换后 `GetActive` 返回新页面 |
| `TestContextUpdateAndResolve` | 写入 `@file`/`@sql`/纯字符串 → `ResolveContext` 返回解析后的值 |
| `TestContextFileRefResolvesContent` | `@file` 引用 Resources 下文件，解析为文件内容 |
| `TestContextSQLRefResolvesQuery` | `@sql` 查询 metadata 表，解析为查询结果 |
| `TestPageDeleteCleansUp` | 删除页面后 DB 记录和 store 索引都被清理 |

## 场景 3：多模块联动（scenario_integration_test.go）

覆盖 `db`（dna/memory/loop/loop_runs/skill）的多模块组合使用。

| 测试 | 验证 |
|------|------|
| `TestDNAInsertAndQuery` | 插入多条 DNA → 按 keyword 模糊查询、按 type 过滤 |
| `TestMemoryPageIsolation` | 两个页面各插入记忆 → 互相不可见 |
| `TestMemoryQueryFilters` | keyword/min-importance/limit 组合过滤 |
| `TestLoopSeedAndRunWithMemory` | add seed → start run → 写入 memory → complete → history 查询 |
| `TestSkillDiscoveryAndQuery` | 构建后 skill 已注册 → `ListSkills` 返回 → `GetSkill` 返回详情 |
| `TestLoopAutoInjectionInContext` | 有活跃 run 时 `ResolveContext` 注入 `loop.focus` 和 `loop.available` |

## 场景 4：Hub 集成（scenario_hub_test.go）

覆盖 hub config/client 的全链路，使用 `httptest.NewServer` mock HTTP 服务端。

| 测试 | 验证 |
|------|------|
| `TestHubLoginWritesConfig` | 模拟 token 回调 → config 文件写入正确 |
| `TestHubPushValidatesAndUploads` | 构建 → save 为 ZIP → validate → mock 服务端接收 |
| `TestHubPullDownloadsAndExtracts` | mock 服务端返回 ZIP → 下载 → load 到本地 |
| `TestHubSearchQueriesAPI` | mock 服务端返回搜索结果 → 解析正确 |
| `TestHubConfigFromEnvironment` | `IROLLHUB_URL`/`IROLLHUB_TOKEN` 环境变量优先于 config 文件 |

## 场景 5：错误与边界（scenario_edge_test.go）

覆盖非法输入、不存在的资源、并发安全。

| 测试 | 验证 |
|------|------|
| `TestBuildWithInvalidSQLFails` | MIGRATE 引用不存在的 SQL 或语法错误 → 构建失败，无残留 |
| `TestBuildWithInvalidBookFails` | 无效 book bundle → 构建失败，不污染 store |
| `TestBuildWithInvalidSkillFails` | skill.md 缺少 name/description → 构建失败 |
| `TestDuplicateBuildRejected` | 同名 iroll 已存在 → 第二次构建被拒绝 |
| `TestQueryNonexistentReturnsError` | 查不存在的 skill/book/loop/dna → 明确错误 |
| `TestContextSQLInjectionSafe` | `@sql` 中包含危险 SQL → 不破坏数据 |
| `TestConcurrentPageOperations` | 两个 goroutine 同时操作不同 page → 互不影响 |
| `TestConcurrentBuildRejectsRace` | 并发构建同名 iroll → 只有一个成功 |

## 已知问题修复

在实现测试时同步修复：

1. **Windows 跨盘符测试**：`TestOpenActiveLoopResolvesAbsoluteCwdAndReturnsContext` 中 `filepath.Rel` 无法处理 C: → D: 跨盘路径，改为在临时目录内操作
2. **Windows symlink 跳过**：6 个 symlink 测试在 Windows 上跳过，保持现状（使用 junction point 作为降级方案，`symlinkOrSkip` 已处理）

## 不做的事

- 不引入 mock 框架 — 继续使用真实 SQLite
- 不测 `cmd` 包的 cobra 命令接线 — 那是单元测试的事，e2e 测业务逻辑
- 不测 `irollhub-web` 前端
- 不测 `outputJSON`/`outputError` 等输出格式化函数

## 预计规模

- **testenv**: 1 个文件，约 80 行
- **5 个场景**: 约 35 个测试函数，总计约 800-1000 行
- **运行时间**: 预计 10-15 秒（大部分是 SQLite + 文件操作）
