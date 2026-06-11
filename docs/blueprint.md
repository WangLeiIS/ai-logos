# Logos 蓝图

## 一句话

Logos 是 AI Agent 的外脑 — 不提供智能，只提供持久化、结构和检索。Agent 负责思考，Logos 负责记住。

## 核心原则

**里面不集成任何 agent 的能力。让 agent 用我们。**

Logos 是一个纯粹的状态管理工具。它不调用 LLM、不生成文本、不做推理。所有智能行为都由外部 agent 完成，Logos 只负责存取。

## 思维模型：像管理 Docker 一样管理 Agent

如果你熟悉 Docker，理解 Logos 只需要一张对照表：

| Docker | Logos | 说明 |
|--------|-------|------|
| Dockerfile | Layerfile | 定义如何构建，指令式（FROM / MIGRATE / COPY） |
| Image | `.iroll` 包 | 构建产物，不可变的出厂设置 |
| `docker build` | `logos roll build` | 从定义文件构建产物 |
| `docker load` | `logos roll load` | 加载产物到本地 |
| Container | Page | 一次运行实例，拥有自己的状态 |
| `docker run` | `logos page new` | 从包创建一个运行实例 |
| Volume | ai_roll.db + Resources/ | 持久化数据 |
| Layer | 构建历史 | 分层叠加，可追溯 |
| Registry | 文件共享 | `.iroll` 包可以分发给其他人 |

核心映射：

```
Dockerfile ──docker build──→ Image ──docker run──→ Container
                                     ↓
                              运行时读写 Volume
                              停止后状态保留在 Volume

Layerfile ──logos build──→ .iroll ──logos load──→ ~/.iroll/
                                                  ↓
                                          logos page new → Page
                                                  ↓
                                          对话中读写 context / memory
                                          对话结束状态保留在 ai_roll.db
```

**关键区别：** Docker 容器是无状态的（删了就没了），但 Page 是有记忆的。每次对话都在前一次的基础上累积。Agent 不是从零启动的容器，而是带着经历成长的生命。

**继承和分发也像 Docker：** 一个 `.iroll` 包可以 `FROM` 另一个包，就像 `FROM ubuntu`。你可以在基础 agent 之上叠加专业知识和技能，构建出专业化的 agent。构建好的 `.iroll` 包可以分发给其他人，他们加载后就能获得一个完整的、开箱即用的 agent。

## 五块拼图

一个完整的 agent 由五个维度定义，每个维度对应 Logos 中的一个模块：

| 维度 | 模块 | 类比 | 存储位置 |
|------|------|------|----------|
| 性格 | dna | 价值观 | ai_roll.db `dna` 表 |
| 习惯 | loop | 条件反射 | ai_roll.db `loop` + `loop_runs` 表 |
| 经历 | memory | 日记 | ai_roll.db `memory` 表；`forget` 尚未实现 |
| 知识 | book | 书架 | Resources/books/ |
| 能力 | skill | 手艺 | Resources/skills/（协议方向，尚未自动注册或发现） |

它们的关系：

```
          ┌──────────────────────────────────┐
          │           context                │
          │  (当前对话的工作记忆)               │
          │                                  │
          │   ┌─── dna（决策时参考）            │
          │   ├─── loop（自主选择的行为种子）    │
          │   ├─── memory（回忆过去的经历）     │
          │   ├─── book（查阅相关知识）         │
          │   └─── skill（调用具体能力）        │
          └──────────────────────────────────┘
```

所有模块通过 **context** 汇聚，但注入方式是**摘要注入，按需加载**：

- context 中只注入各模块的摘要或索引（如 dna 只加载 question 不加载 answer，skill 只注入 description 列表）
- agent 需要详细信息时，通过 CLI 按需查询（query-dna、book query、加载 skill.md）
- 这保证 context 始终精简，不随模块增长而膨胀

Agent 在每次对话开始时读取 context，就获得了「我是谁、我在做什么、我知道什么、我能做什么」的完整概览。详情按需获取，不一次性塞满。

## 生命周期

### 阶段一：构建（Build）

创建或更新一个 `.iroll` 包。这一步由开发者完成，agent 不参与。

```
Layerfile + 资源文件 ──logos roll build──→ .iroll 包
```

Layerfile 只有三条指令：`FROM`（继承基础层）、`MIGRATE`（执行 SQL）、`COPY`（复制资源）。当前构建完成后会自动校验并注册 book；skill 注册尚未实现。

构建产出的 `.iroll` 包就是 agent 的「出厂设置」— 包含初始的人格（dna）、习惯（loop）、知识（book）以及可选的原始 skill 资源，但还没有任何记忆。当前运行时不会自动注册或发现 skill。

### 阶段二：加载（Load）

将 `.iroll` 包安装到本地工作目录。

```
agent.iroll ──logos roll load──→ ~/.iroll/agent-name/
```

同一个 `.iroll` 包可以加载到多个工作目录，互不干扰。每个工作目录维护自己的活跃页面。

### 阶段三：对话（Session）

每次新对话，agent 通过三步启动：

```
1. logos page new <name> --cwd .    → 创建页面，继承模板 context
2. logos page get-context --cwd .   → 读取解析后的 context
3. 按照 context 中的指令行事
```

**启动后 agent 获得了什么：**

- `system_prompt`：我是谁，怎么说话
- `description`：我的职责描述（来自 metadata）
- `dna`：我的决策维度和选择倾向（摘要：仅 question，answer 按需查询）
- `loop`：我需要关注的循环任务（focus = 正在执行的，available = 可用的）
- （未来）`skills`：我能调用的能力列表（摘要：仅 name + description）；当前尚未注入
- 以及任何通过 `@file` 和 `@sql` 注入的自定义字段

**对话过程中：**

- 需要做决策时 → `logos page query-dna` 查询 DNA
- 需要查知识时 → `logos book query` 检索书籍
- 需要用能力时 → 加载对应 skill.md 执行
- 需要回忆时 → `logos page query-memory` 查询当前 page 的记忆
- context 变化时 → `logos page update-context` 更新上下文
- 自主选择要做的事情时 → `logos loop run/update/complete/abort` 记录过程

### 阶段四：记忆管理（Memory Lifecycle）

对话不是孤立的。随着对话累积，agent 的状态持续演化。

未来的 context 压缩能力会在对话增长接近上限时执行：

```
对话增长
  ↓
context 接近最大值？
  ├─ 否 → 继续
  └─ 是 → 快照存入 memory → context 压缩为摘要 → 继续
```

该能力尚未实现。目标是让压缩保留关键结构，并将完整快照写入 memory，后续仍可通过 `query-memory` 检索。

**每个 page 的记忆是隔离的。** Page A 的 memory 和 Page B 互不影响。同一个 agent 在不同项目中工作，各自积累各自的经历。

未来可以由 agent 自主选择 `sleep` loop 来整理记忆：

```
sleep 整理：
  memory（完整的经历记录）
    → 提炼核心结构，保留在 memory
    → 次要细节移入 forget
```

`forget` 表尚未实现。设计目标是保留被移出的原始细节，需要时可检索恢复，而不是直接删除。

### 阶段五：演进（Evolution）

agent 不是静态的。通过分层构建，可以迭代升级：

```
v1: base-agent.iroll           → 基础人格 + 通用能力
v2: FROM base-agent + 扩展     → 继承 v1，追加新技能
v3: FROM v2 + 迁移             → 继承 v2，升级数据结构
```

每次构建都有历史记录（history 表），可以追溯演进过程。设计目标是让 memory 和 loop_runs 跨版本保留，使 agent 在升级后不会失去记忆。

## 与 Agent 的合作模式

Logos 定义了 agent 和状态管理之间的边界：

```
┌─────────────┐                    ┌─────────────┐
│             │   CLI 调用         │             │
│   Agent     │ ──────────────────→ │   Logos     │
│  (思考者)    │                    │  (记忆者)    │
│             │ ←────────────────── │             │
│             │   结构化数据        │             │
└─────────────┘                    └─────────────┘
```

**Agent 负责：**
- 理解用户意图
- 决定调用哪个 skill
- 判断是否需要记住某件事
- 执行 loop 中的任务
- 整理 memory（sleep 循环）
- 生成回答

**Logos 负责：**
- 持久化存储（SQLite + 文件系统）
- 结构化检索（SQL 查询、标签匹配）
- Context 解析（@file、@sql 自动解析）
- 状态隔离（每个页面独立的运行状态）
- 数据安全（路径校验、ZIP 防护）
- 包管理（构建、加载、导出、版本追踪）

**它们不互相依赖：** Logos 可以独立运行，不关心调用者是 GPT、Claude 还是本地模型。Agent 可以不使用 Logos，但就无法持久化状态。两者通过 CLI 这个薄接口连接，各自独立演进。

## 数据流总览

```
                         构建
                          │
                     .iroll 包
                          │
                    logos roll load
                          │
                     ~/.iroll/<name>/
                     ├── ai_roll.db
                     └── Resources/
                          │
              ┌───────────┼───────────┐
              │           │           │
         logos page new   │      logos page
              │           │      get-context
         创建页面         │           │
         继承模板         │     解析 @file @sql
              │           │     注入 loop context
              │           │           │
              │           │      context JSON
              │           │      ┌────┴────┐
              │           │      │ agent   │
              │           │      │ 读取并  │
              │           │      │ 执行    │
              │           │      └────┬────┘
              │           │           │
         logos page       │     对话过程中的读写：
         query-memory     │     ├─ query-dna（决策）
         update-context   │     ├─ book query（知识）
         loop run ...      │     ├─ query-memory（回忆）
              │           │     ├─ update-context（更新）
              │           │     └─ loop run（任务执行）
              │           │
              └───────────┼───────────┘
                          │
                     记忆生命周期
                     ├─ 已实现：page 隔离的 memory 查询
                     └─ 未实现：context 压缩、forget 归档
```

## 设计哲学

1. **Agent 不应该重复思考同一件事。** 通过 memory 和 context，让 agent 站在昨天的肩膀上。
2. **Agent 不应该在每轮对话中重新认识自己。** 通过 dna 和 context，人格是持久且一致的。
3. **Agent 应该知道自己可以做什么。** 通过 loop context 自主选择，并用 loop_runs 记录过程。
4. **Agent 应该有可控的知识边界。** 通过 book，知识是注册的、可校验的、可溯源的。
5. **Agent 的能力应该是可发现、可组合的。** 通过 skill，能力按需加载，不污染 context。
6. **遗忘是健康的。** 未来通过 forget，让记忆不过载，但不丢失。
7. **摘要注入，按需加载。** Context 中只放概览，详情通过 CLI 按需获取。信息越多，这条越重要。
8. **信任包的制作者。** .iroll 包中的 @sql 是裸 SQL，skill 中的脚本是全权限。包的制作者和运行者之间没有权限隔离 — 这是有意为之的简化。如果你不信任一个包，不要加载它。

## 已知边界

当前版本的一些有意识取舍，留待后续迭代：

- **Loop 永远由 agent 自主执行。** Logos 不调度、不规划、不执行，只提供种子、当前 focus 和生命记录。
- **Memory 写入入口尚未接入 context 压缩。** 当前已有 page 隔离的查询与 DB 写入/整理 API，但没有自动压缩流程。
- **Forget 尚未实现。** 记忆整理目前没有归档次要细节的持久化目标。
- **Skill 注册与查询尚未实现。** `Resources/skills/` 是协议方向，不是当前运行时能力。
- **Logos CLI 尚未接入 irollhub。** irollhub API 服务已经存在，但 `login/push/pull/search` 仍是未来命令。
- **@sql 无权限隔离。** Context 中的 SQL 查询直接执行，不做安全沙箱。包的制作者拥有完全信任。

详细技术规格见 [rebot-roll.md](rebot-roll.md)。
