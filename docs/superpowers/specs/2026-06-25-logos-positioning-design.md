# Logos 定位与交互模型设计

> 2026-06-25 · 从 Logos、用户、AI Agent 三者的根本关系出发，重新明确 Logos 的边界和定位。

## 1. Logos 是什么

Logos 是一个 **结构化的 Agent 状态存储系统**。它提供标准化 `.iroll` 包格式存储 Agent 的人格、记忆、行为种子、知识和资源。

**核心原则：不集成任何 Agent 的能力。让 Agent 用我们。**

Logos **不是**：
- runtime — 不执行、不调度、不规划
- chat platform — 不管理对话流
- task manager — 不分配工作、不追踪 deadline

Logos **是**：
- Agent 人格（dna, metadata）的持久层
- Agent 行为模式（loop seeds）的种子库
- Agent 记忆（memory）的 Q&A 索引
- Agent 运行记录（loop_runs）的事实来源
- Agent 知识（book, skill）的注册中心
- **Agent 跨 session 状态（user_context）的承载者**

## 2. 三者关系

```
用户 ──自然语言──▶ AI Agent ──CLI 调用──▶ Logos
  │                   │                    │
  │                   │                    ├─ 人格 (dna, metadata)
  │                   │                    ├─ 能力 (loop seeds, skills)
  │                   │                    ├─ 知识 (books, memory)
  │                   │                    └─ 状态 (pages.context, loop_runs)
  │                   │
  └── 完全透明 ────────┘
     用户不碰 Logos，不感知 Logos
```

- **用户**：只对 Agent 说话。不知道 Logos 的存在。
- **AI Agent**：Logos 的唯一使用者。启动时加载 context 获取人格和状态，运行中调用 CLI 记录状态变化，结束时写回 user_context。
- **Logos**：被动存储和查询。Agent 用的时候响应，不用的时候沉默。

## 3. 交互周期

### 3.1 Session 启动

```
Agent 启动
  └─ logos page new (首次) 或 logos page get-context (已有 page)
       └─ 获取：人格指令 (system_prompt)、DNA、可用 loop seeds、当前 user_context、活跃 loop run
```

### 3.2 Session 中

```
用户请求 ──▶ Agent
  ├─ 在 loop seeds 中搜索匹配的原型
  │   ├─ 有原型 ──→ logos loop run → update → complete
  │   └─ 无原型 ──→ Agent 判断是否值得沉淀
  │       ├─ 值得 ──→ logos loop add → loop run → update → complete
  │       └─ 一次性 ──→ 直接执行，不建 seed

Agent 自主行为 ──→ Agent 选择 loop seed → logos loop run → ...

Agent 随时：
  ├─ logos page update-context (写回 user_context)
  ├─ logos page query-memory (检索记忆)
  └─ logos page query-dna (查询决策基因)
```

**关键洞察：用户任务是 loop run 的触发源，不是独立概念。**

loop run 不区分 "用户来的" 还是 "Agent 自己想的" — 它只是 run。种子不存在时，Agent 可以 `loop add` 创建新种子再启动，这是用户任务沉淀为可复用模式的过程。

### 3.3 Session 结束

user_context 保留在 `pages.context` 中。下次 session 启动时，Agent 通过 `get-context` 自动恢复工作状态。

```
Session N:
  user_context: {"project": "个人博客", "done": ["首页"], "todo": ["部署"]}

Session N+1:
  get-context → user_context → "我在做个人博客，首页已完成，待部署" → 直接继续
```

## 4. 多米诺骨牌：连续、长程、稳定

Logos 的设计追求**最小触发 → 链式反应 → 稳定循环**：

```
template context 预置 "user_context": {}
        │
        ▼
Agent get-context 读到 user_context
        │
        ▼
Agent 知道"上次在哪" → 对话 → 执行任务 → loop run
        │
        ▼
Agent update-context 写回 user_context
        │
        ▼
下次 session，get-context → user_context 告诉新 Agent "我们在这里"
        │
        ▼
... 无限循环
```

Logos 做的：**提供一个空位，记住你放进去的东西。**

Agent 做的：**决定放什么、什么时候放、怎么解释。**

## 5. 数据边界

| 概念 | 属于 | 原因 |
|---|---|---|
| Agent 人格 (dna, system_prompt) | Logos (inner.db) | 跨 session 不变，构建时写入 |
| Agent 行为模式 (loop seeds) | Logos (inner.db) | 可复用原型 |
| Agent 运行记录 (loop_runs) | Logos (outer.db) | 不可变事实记录 |
| 从对话提炼的知识 (memory) | Logos (outer.db) | Q&A 索引，可检索 |
| 跨 session 状态 (user_context) | Logos (pages.context) | 会话连续性 |
| 原始对话日志 (chat_history) | **Agent runtime** | Agent 自己管理，Logos 不存 |

**chat_history 不做。** 对话日志是 Agent runtime 的责任——Agent 的 context window 就是它的短期记忆。Logos 存储的是提炼过的东西（memory），不是原始日志。

## 6. 变更范围

### 需要改的

| 文件 | 变更 | 原因 |
|---|---|---|
| `examples/base-agent/init_data.sql` | 模板 page (`page_id='0'`) 的 context JSON 加 `"user_context":{}` | 给 Agent 一个空白的约定字段，让 Agent 知道"这里可以写" |

模板 page_id='0' 的 context 从：

```json
{"system_prompt":{...}, "response_contract":{...}, "dna":{...}}
```

变为：

```json
{"system_prompt":{...}, "response_contract":{...}, "dna":{...}, "user_context":{}}
```

### 不需要改的

- ❌ 不建新表（无 chat_history、无 user_task、无 user_context 表）
- ❌ 不建新 CLI 命令（现有 `update-context`/`get-context` 完全覆盖）
- ❌ 不定义 user_context 的 schema（Agent 自由决定内容）
- ❌ 不改 builder、db、store 层代码
- ❌ 不改 CLI 代码
- ❌ 不改 outer.db / inner.db 结构

## 7. Agent 行为指南（写入 skill.md 的参考）

这些内容属于 `logos-1` skill 的更新范围，不是本次 spec 的直接变更，但记录下来作为设计意图：

1. **Session 启动**：`get-context` 后检查 `user_context` — 如果非空，理解上一次的状态并继续。
2. **用户请求到达**：在 loop seeds 中搜索匹配的原型。有则启动 run；无则判断是否值得创建新 seed。
3. **Session 结束前**：`update-context` 写回 `user_context` — 记录当前项目、完成项、待做项等关键状态。
4. **user_context 原则**：只记录"下次 session 需要知道的最少信息"。不存对话日志，不存已完成任务的细节（细节在 memory 和 loop_runs 里）。
