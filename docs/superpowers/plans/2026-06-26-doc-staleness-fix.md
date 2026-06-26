# 文档脱节统一修复计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 9 个项目文档全部对齐到当前代码真相，消除会误导 agent/开发者的硬错误（错命令名、错架构、宣称已实现功能"未实现"）。

**Architecture:** 纯文档修订（一个微小的 init_data.sql 代码改动除外）。每个 task 对应一个文件，subagent 拿"当前真相参考块"+ 该文件的审计发现，重写失真章节。所有 task 共用同一份"当前真相"，保证统一。验证靠 grep 残留术语 + 代码任务跑测试。

**Tech Stack:** Markdown 文档 + 一处 Go 项目的 SQL seed。

**审计依据:** 本会话三个并行审计 agent 的输出（findings 已并入各 task）。

---

## 当前真相参考块（所有 task 共用，subagent prompt 必须内嵌）

> **DB 架构（最重要）：** `.iroll` ZIP 包含 `roll-inner.db` + `roll-outer.db` + `Resources/` + `layer.json`（无 `ai_roll.db`）。
> - `roll-inner.db`（只读蓝图，构建时写入）：`metadata` / `dna` / `loop`（种子）/ `skill` / `book` / `history` / + 模板行（`pages`/`memory`/`loop_runs` 中 `page_id='0'` 的）。
> - `roll-outer.db`（模板）：`pages`/`memory`/`loop_runs` 的 schema。运行时按 cwd 复制到 `<cwd>/.iroll/<name>.db`（自定义 cwd）或 `~/.iroll/<name>/<version>/workspace/.<name>.outer.db`（默认 workspace）。
> - 打开方式：以 outer 为主库，`ATTACH roll-inner.db AS inner`。裸表名 = outer；`inner.` 前缀 = inner。
> - 全局 `~/.iroll/system.db`：`page_index` / `active_page` / `config`。

> **命令面：**
> - `page`：`new` / `list` / `switch` / `delete` / `default`（set/show/clear）/ `get [path]` / `set <path> <value> | --content` / `unset <path>` / `alias <name> | --clear` / `query [sql]` / `query-memory` / `query-dna`。（`page current` 已删除；`get-context`→`page get`；`update-context`→`page set`。）
> - `loop`（扁平，非分组）：`list` / `inspect` / `add` / `edit` / `remove` / `archive` / `restore` / `run` / `update` / `complete` / `abort` / `reflect` / **`ps`**（不是 `current`）/ `history` / `show`。
> - `roll`：`build` / `load` / `list` / `rm` / `save` / `inspect` / `history` / `evolving` + hub：`login` / `logout` / `push` / `pull` / `search`。（构建命令是 `logos roll build`，不是 `logos build`。）
> - `book`：`list` / `inspect` / `query`。`skill`：`list` / `show`。`status`。

> **Irollfile 指令（4 条，不是 3 条）：** `FROM <name>` / `MIGRATE <file>`（→ inner）/ `MIGRATE OUTER <file>`（→ outer 模板）/ `COPY <src> <dest>`。按文件顺序执行，不分组。

> **输出协议（三段式）：** page/loop/evolving/query-dna/memory 用新格式：stdout 打印 `{"status":"ok"}` + 可选 data 行 + 可选 `{"hints":[{"action":"...","cmd":"..."}]}`；错误 `{"status":"error","code":"...","error":"..."}` + 可选 hints，然后 `os.Exit(1)`。错误码：`invalid_tag` / `iroll_not_found` / `no_default_page` / `no_active_page` / `page_not_found` / `db_open_failed` / `internal` / `key_not_found`。**注意：`book`/`status`/`skill`/所有 `roll*` 命令仍是旧单行格式（本轮未改）—— 文档描述三段式时不要声称"所有命令统一"。**

> **其他事实：**
> - `schema_version` = 2（不是 1）。
> - `loop` 表有 `type`（`auto`/`normal`，`auto` 在 `page new` 时自启）和 `describe` 列。
> - `pages` 表有 `alias` 列。
> - `system.db` 的 `page_index` 和 `active_page` 都有 `iroll_version` 和 `outer_db_path` 列；`page_index` 还有 `alias`。
> - `loop_runs` 是 **page 隔离**（每行有 `page_id`）；`loop` **种子**是 roll 级（inner，page 无关）。别写反。
> - `skill` **已完整实现**：构建时发现/校验/注册 + `skill list`/`skill show` CLI。绝不能再写"未实现"。
> - `forget` 表 / context 压缩：**未实现**（未来方向）。
> - `user_context`：跨 session 状态约定键（positioning spec），**尚未加入模板**（Task 10 会加）。
> - `@sql` 查 inner 表必须带 `inner.` 前缀（如 `SELECT value FROM inner.metadata WHERE ...`），裸名查的是 outer。

## 验证：残留术语 grep 清单（每个 task 后跑相关项；全部完成后跑全量）

```bash
# 以下在 "当前文档"（非 docs/superpowers 历史记录）中应为 0 命中：
grep -rn "ai_roll\.db\|get-context\|update-context\|page current\|loop current" \
  --include="*.md" CLAUDE.md README.md docs/blueprint.md docs/rebot-roll.md \
  docs/iroll-protocol-v1.md docs/iroll-layered-build-spec.md docs/logos-e2e-testing.md docs/todo.md \
  skills/logos-1/skill.md
# "仅三条指令"/"三条指令" 应为 0（除非明确说"四条"）
grep -rn "三条指令\|schema_version.*[^0-9]1\b\|skill.*未实现\|skill.*尚未" \
  --include="*.md" CLAUDE.md README.md docs/ skills/logos-1/skill.md
```
（`docs/superpowers/*` 是带日期的历史记录，CLAUDE.md 明确说可能含已替代的术语 —— 不改。）

---

## Task 1: CLAUDE.md（最高优先级，每 session 加载）

**File:** `CLAUDE.md`

**审计发现（必须全部修正）：**
- L7 "ZIP archive + SQLite database"（单数）→ 两库。
- L75 `db/` 描述漏 skill/book/history，且没说跨 inner/outer 两库。
- L81 "ZIP archive containing `ai_roll.db` and `Resources/`" → `roll-inner.db` + `roll-outer.db` + `Resources/`。
- L86 `@sql` "queries ai_roll.db" → 查 inner（`inner.` 前缀），裸名查 outer。
- **L88 `loop_runs` 描述写反**："page-independent execution state" → 实际 **page 隔离**（每行 page_id）；`loop` 种子才是 page 无关。
- L91-99 "Database Structure (ai_roll.db per iroll)" + 表清单 → 拆成 inner 表（metadata/dna/loop/book/skill/history + 模板行）和 outer 表（pages/memory/loop_runs）；**补 `skill` 表**。
- L139 `BuildLoopContext()` + `get-context` → 注入发生在 `db.ResolveContext`（`page get`/`page new` 时），注入键是顶层 `loop_focus`/`loop_available`。

**做法：** 把 "Important Implementation Details" 里的 DB 结构、Context Resolution、Loop Run Lifecycle 段对齐当前真相。命令示例（L33-36）保持已改的 `page get`。

- [ ] 重写 CLAUDE.md 的 Project Overview / Database Structure / Context Resolution / Loop 段
- [ ] grep 验证 CLAUDE.md 无 `ai_roll.db`/`get-context`/`loop_runs.*independent` 残留
- [ ] commit: `docs: align CLAUDE.md with inner/outer split and current commands`

---

## Task 2: README.md

**File:** `README.md`

**审计发现：**
- L32 "SQLite 数据库（`ai_roll.db`）" → 两库。
- L46 `@sql` "查询 ai_roll.db" → inner（`inner.` 前缀）。
- **L71 "动态注入 ... `loop.focus` 和 ... `loop.available`"** → 实际注入**顶层** `loop_focus` / `loop_available`（扁平，非 nested `loop.`）。
- L113-125 页面管理命令块：补 `page default`；`page alias` 清除语法是 `page alias --clear`。
- **L138-146 Loop 管理块：`loop current`（L145）不存在 → `loop ps`**；`loop list` 补 `--stats`；`loop add` 补 `--type`。
- L160-171 "数据库结构 ai_roll.db" → 拆 inner/outer；`loop` 行补 `type`/`describe`；补 `skill` 表。
- L173-179 system.db：补 `outer_db_path`/`iroll_version`/`alias` 列说明。
- 全文：补一段"输出格式（三段式）"说明（仅描述 page/loop/evolving 等用三段式，不声称全统一）；补 `skill` 命令组到 CLI 列表。

- [ ] 重写 README 的 iroll 包/Context/Loop/数据库结构/CLI 命令段
- [ ] grep 验证无 `ai_roll.db`/`loop current`/`get-context`/`loop.focus`（应 `loop_focus`）
- [ ] commit: `docs: align README with current DB split, commands, and output format`

---

## Task 3: docs/rebot-roll.md（声明为权威文档之一）

**File:** `docs/rebot-roll.md`（§5.3 命令表已部分更新，但 §3/§4/§5 其余段未动 —— 查全文）

**审计发现：**
- L2/§3 标题 "数据库结构 ai_roll.db" → 拆 inner/outer。
- L19 ZIP 布局 `ai_roll.db` → `roll-inner.db` + `roll-outer.db`。
- L22-24 "数据库分为四个部分" → 按 inner/outer 重述。
- L50-56 dna `type` "决策维度：认知观/伦理观/..." → 实际 `type` 是自由字符串（示例用 `idea`/`emotion`），无四维 CHECK。
- L58-72 loop 种子表：补 `type` 列；说明 `auto` 自启。
- L99-108 pages 表：补 `alias` 列。
- L192-210 system.db：`page_index`/`active_page` 补 `iroll_version`/`outer_db_path`；`page_index` 补 `alias`；说明 `config` 存 `default_page:<name>`。
- **L255-263 §5.4 "Irollfile 仅三条指令" → 四条，补 `MIGRATE OUTER`。**
- L265-279 §5.5 Loop：**`loop current`（L276）→ `loop ps`**；`loop add` 补 `--type`；`loop list` 补 `--stats`；`loop history <name>` 带 `--page`/`--limit`。
- L222-232 §5.2 roll 命令：补 `roll evolving` + hub 命令（login/logout/push/pull/search）。
- L236-249 §5.3 page 命令：补 `page default`；说明 get/set/unset/alias/query 还接受 `--alias`/`--roll`。
- L330-385 §8 memory 生命周期：`forget` 表标"未实现"是对的，保留；确保措辞是"未来设计"。
- L306-321 §7 路线图：CLI 清单补 `skill` 和 hub 命令。

- [ ] 重写 §3（DB 结构）、§5.2/5.4/5.5（命令 + Irollfile 指令）、system.db 表、dna/loop/pages schema
- [ ] grep 验证
- [ ] commit: `docs: align rebot-roll.md with inner/outer split, 4 Irollfile instructions, loop ps`

---

## Task 4: skills/logos-1/skill.md（agent 技能文档，照做必错的硬错误）

**File:** `skills/logos-1/skill.md`

**审计发现（硬错误优先）：**
- **L128 `logos loop current --cwd .` → `logos loop ps --cwd .`**（命令不存在，agent 照做必报错）。
- **Command Reference 表 L167 `...|current|history|show` → 把 `current` 改 `ps`。**
- **L176 Key Concepts 的 @sql 示例 `SELECT value FROM metadata WHERE key='description'` → 必须带 `inner.` 前缀：`SELECT value FROM inner.metadata WHERE ...`**（裸名查 outer 会失败）。
- L4-9 frontmatter：补 `page set`（per-key）/`page unset`/`page alias`/`page query`/loop/三段式输出。
- L57 "typically contains a `system_prompt`" → 也含 `response_contract`/`dna`。
- L116-118 book 命令：补一句说明 book 仍输出旧单行 JSON（非三段式），agent 解析需注意。
- Command Reference：`page get/set` 补 `--roll`/`--alias`；`page query` 补 `--page`/`--alias`/`--dry-run`；补 `page default`/`skill`/`roll evolving`/hub 行。
- L171 "iroll — ... (database + resources)" 单数 → 两库。

- [ ] 修 loop current→ps（2 处）、@sql inner. 前缀、frontmatter、Command Reference 表
- [ ] grep 验证无 `loop current`/`get-context`
- [ ] commit: `docs: fix skill.md hard errors (loop ps, inner. prefix) and expand command ref`

---

## Task 5: docs/blueprint.md（设计哲学文档，5 处宣称 skill 未实现）

**File:** `docs/blueprint.md`

**审计发现（最严重的是 skill 误报）：**
- **L59/94/96/124/269 宣称 skill "尚未实现/未自动注册" → 全部改为"已实现"**（构建时发现/校验/注册 + CLI）。这是最误导的失真。
- L25 Docker 对比 "ai_roll.db + Resources/" → `roll-inner.db` + `roll-outer.db` + `Resources/`。
- L42 数据流 "状态保留在 ai_roll.db" → `roll-outer.db`（per-cwd）。
- L53-59 五块拼图表"存储位置 ai_roll.db" → dna/loop 种子在 inner；memory/loop_runs 在 outer；skill 改"已实现"。
- L114 `logos page get-context` → `logos page get`。
- L133 `logos page update-context` → `logos page set`。
- L219-243 数据流总览 ASCII：`ai_roll.db` → 两库；`get-context`/`update-context` → 新名。
- L19 Irollfile "FROM/MIGRATE/COPY" → 补 `MIGRATE OUTER`。
- L165-175 Evolution 段"memory/loop_runs 跨版本保留"：澄清 loop_runs 在 per-cwd outer，不随 iroll 版本继承（只继承 inner 蓝图）。

**做法：** 保留 blueprint 的设计哲学叙事，只修正事实错误。skill 那 5 处必须改。

- [ ] 修 skill 5 处、DB 名、命令名、Irollfile 指令
- [ ] grep 验证无 `skill.*尚未`/`ai_roll.db`/`get-context`
- [ ] commit: `docs: correct blueprint.md — skill is implemented, DB split, current commands`

---

## Task 6: docs/iroll-protocol-v1.md（标"冻结 v1"但已不匹配）

**File:** `docs/iroll-protocol-v1.md`

**审计发现：** 整篇描述旧世界（单库/3 指令/schema_v1/旧命令名）。

**做法（banner + 修硬错误，不全文重写）：**
- 顶部加醒目 banner：`> ⚠️ 本文档描述 v1 协议，部分已被后续设计取代（DB 拆分、MIGRATE OUTER、命令改名、schema_version=2）。当前实现以 README.md 和 docs/rebot-roll.md 为准。`
- L11-25 ZIP 布局 `ai_roll.db` → 说明现为 `roll-inner.db` + `roll-outer.db`。
- L33-43 layer.json `schema_version: 1` → 2。
- L115-137 loop 表 → 补 `type` 列。
- L171-204 loop_runs：删除不存在的 `idx_loop_runs_one_active_main` 索引说明（实际未建）。
- L245-265 pages 表 → 补 `alias`。
- L362/376-396 `get-context`/`update-context` → `page get`/`page set`。
- L445-462 §5 Irollfile "仅三条" → 四条，补 `MIGRATE OUTER`；纠正执行顺序是文件顺序。
- L466-497 §6 system.db → 补 `iroll_version`/`outer_db_path`/`alias` 列。

- [ ] 加 banner + 修上述事实错误
- [ ] grep 验证（banner 外无 `ai_roll.db`/`get-context`/`schema_version.*1`）
- [ ] commit: `docs: add superseded banner to iroll-protocol-v1.md, fix dangerous errors`

---

## Task 7: docs/iroll-layered-build-spec.md（构建示例会失败的硬错误）

**File:** `docs/iroll-layered-build-spec.md`

**审计发现（示例 SQL 会失败的优先）：**
- L506-540 多处 `INSERT INTO memory(content, created_at, importance)` → 现 schema 要求 `page_id`/`name`/`question`（NOT NULL），**这些示例会失败**，必须改成合法 schema。
- L485-500 `skill(name, description, file_path, ...)` → 实际列名是 `path` 不是 `file_path`。
- L366-367 `logos build -f Irollfile -t ...` → `logos roll build ...`。
- L105/116-120/593 "三条指令" → 四条，补 `MIGRATE OUTER`。
- L37-49/64-67 `ai_roll.db` → 两库；"只包含本层新增数据"不成立（`processFrom` 整库复制）。
- L73-96 layer.json 字段 `author/migration/resources_hash` → 实际只有 `layer_id/parent/description/created_at/schema_version`；schema_version=2。
- L319-346 §5.1/5.2 "content-addressed layer 缓存 / 不重复存储" → **该架构未实现**，改为说明实际直接构建到 `~/.iroll/<name>/<version>/`。
- L347-355 §5.3 registry 命令 → 改为 irollhub 实际语法 `logos roll push <file|name> <org>/<pkg>:<ver>`。
- L416-449 §7 init_schema 示例 → 对齐真实 schema（memory 带 page_id/name/question；无独立 context 表，是 pages.context；history 由代码建）。
- L566-583 §8 "与现有 CLI 关系" 列的 `session init/list`、`get-context`、`add-memory` → 都不存在，删除或改为现状。

**做法：** 修所有会失败的示例 + 命令名 + 架构描述。对未实现的 layer 缓存段，改为"当前直接构建，未做内容寻址缓存"。

- [ ] 修示例 SQL、skill 列名、build 命令、Irollfile、layer.json 字段、layer 缓存段、§8 命令
- [ ] grep 验证无 `logos build -`（非 roll）/`file_path`/`三条指令`
- [ ] commit: `docs: fix iroll-layered-build-spec — valid SQL examples, roll build, MIGRATE OUTER`

---

## Task 8: docs/logos-e2e-testing.md + docs/todo.md（轻量）

**Files:** `docs/logos-e2e-testing.md`, `docs/todo.md`

**e2e-testing 审计：**
- **L9 `logos page get-context` → `logos page get`**（硬错误）。
- L5 `logos roll build -t cat:base .` → 对齐实际 flag（`-f <Irollfile> -t <name>`）。
- 全文过薄：补一句指向真实 e2e 框架 `iroll/e2e/`（`scenario_*_test.go` + `testenv`）。

**todo.md 审计：**
- L7 "冻结 .iroll 格式 [x]" → 改为说明格式已演进（拆库/type/alias/MIGRATE OUTER/schema_v2），冻结已解除或更新措辞。
- 补已上线但未列的功能：DB 拆分、三段式输出、page 重组（get/set/unset/alias/query/default）、loop 全命令、skill/book CLI、MIGRATE OUTER、query-dna/query-memory、page alias/default 机制。
- L42 "决定前端优先级" → 前端已上线（L35 [x]），措辞调整。

- [ ] 修 e2e-testing 的 get-context + build flag + 指向真实框架
- [ ] 更新 todo.md 路线图（解冻说明 + 补已上线项）
- [ ] commit: `docs: update e2e-testing and todo roadmap to current state`

---

## Task 9: user_context 加入模板（唯一代码改动）

**File:** `examples/base-agent/init_data.sql`

**背景：** positioning spec（`docs/superpowers/specs/2026-06-25-logos-positioning-design.md` §6）说模板 page context 要加 `"user_context":{}` 约定键，但一直没做。这让 skill.md 的 `page set user_context.project blog` 示例有约定支撑。

**改动：** 在 `init_data.sql` L114-118 的模板 page context JSON 里加一行 `"user_context":{},`：

```sql
        '{' ||
            '"system_prompt":{"@sql":"SELECT value FROM inner.metadata WHERE key = ''system_prompt''"},' ||
            '"response_contract":{"@sql":"SELECT value FROM inner.metadata WHERE key = ''response_contract''"},' ||
            '"dna":{"@sql":"SELECT name, type, weight, question, answer FROM inner.dna ORDER BY weight DESC"},' ||
            '"user_context":{}' ||
        '}',
```

**验证：** 重构 base-agent 后 `page new` → `page get`，确认返回的 context 含空 `"user_context":{}`。

- [ ] 改 init_data.sql
- [ ] `cd iroll && go test ./db/... ./builder/...`（确认无回归）
- [ ] （可选）smoke：build base-agent → page new → page get 看到 user_context
- [ ] commit: `feat(base-agent): add user_context convention key to template page context`

---

## Task 10: 最终验证

- [ ] 全量 grep（上方"残留术语 grep 清单"）→ 当前文档 0 残留（superpowers 历史记录除外）
- [ ] `cd iroll && go build ./... && go test ./... && go vet ./...` → 全绿（确认 init_data.sql 改动无回归）
- [ ] 抽查：`grep -rn "loop current\|page current\|get-context\|update-context\|ai_roll\.db" --include="*.md" CLAUDE.md README.md docs/ skills/ | grep -v superpowers` → 空
- [ ] （如有多文件连续改动）合并为 final 验证 commit 或仅记录

---

## Self-Review

**覆盖：** 审计的 9 个文档各有 task（1-8 覆盖 CLAUDE/README/rebot-roll/skill/blueprint/protocol/layered-build/e2e-testing/todo）；user_context 代码缺口有 Task 9；最终验证 Task 10。

**硬错误优先级：** skill.md 的 `loop current`/`@sql 缺 inner.`、blueprint 的 skill"未实现"、layered-build 的失败 SQL 示例 —— 这些是"照做必错"的，在各 task 里标了优先。

**范围决定（已在 plan 体现）：**
- 三段式输出**不收尾**（book/status/skill/roll\* 仍旧格式）—— 这是 v2 spec 的明确范围决定，文档只描述 page/loop/evolving 等用三段式，不声称全统一。
- protocol-v1 / layered-build 这类旧 spec 文档：**加 superseded banner + 修硬错误**，不全文重写（保留历史设计叙事）。
- `docs/superpowers/*` 历史记录**不动**（CLAUDE.md 明确说可能含已替代术语）。

**执行方式：** 文档 task 适合 subagent-per-file（互不冲突，可并行）；Task 9 是代码改动单独跑。建议 subagent-driven，review 简化为 grep 验证 + 抽查（文档无需两 stage code review）。
