# Logos — AI Agent 记忆管理系统

## 1. 项目定位

Logos 是一个 AI agent 的记忆管理工具。它提供了一种标准化的包格式 `.iroll`（智能卷轴）来存储 agent 的全部状态：记忆、技能、知识和资源。通过 `logos` 命令行工具，agent 可以加载、构建、管理和共享这些记忆包。

**核心原则：里面不集成任何 agent 的能力。让 agent 用我们。**

## 2. 包格式 .iroll

`.iroll` 是一个 ZIP 归档文件，包含两部分：

```
agent.iroll (ZIP)
├── Resources/              # 资产：图片、音频、文本、脚本、技能文件等
│   ├── scripts/
│   ├── skills/
│   └── books/
└── ai_roll.db              # SQLite 数据库，包含 agent 的全部记忆和知识
```

## 3. 数据库结构 ai_roll.db

数据库分为四个部分，字段不限制，可自由扩展。

### 3.1 自我部分

| 表 | 必须 | 说明 |
|----|------|------|
| metadata | 是 | key-value 元数据（name, version, description 等） |
| dna | 否 | agent 的决策 DNA，Q&A 对定义底层决策机制 |
| loop | 否 | 循环任务表，定义 agent 的运行模式（once/periodic） |

**metadata 表结构：**

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| key | TEXT | NOT NULL | 键 |
| value | TEXT | NOT NULL | 值 |
| remark | TEXT | | 备注 |
| created_at | TEXT | NOT NULL | 创建时间 |
| updated_at | TEXT | NOT NULL | 更新时间 |

**dna 表结构：**

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PRIMARY KEY AUTOINCREMENT | 主键 |
| name | TEXT | NOT NULL | 唯一标识，如 `handle-correction` |
| type | TEXT | NOT NULL | 决策维度：认知观 / 伦理观 / 审美观 / 本体观 |
| question | TEXT | NOT NULL | 决策困境 |
| answer | TEXT | NOT NULL | 这个 agent 的选择 |
| weight | REAL | DEFAULT 0.5 | 权重，越高越核心 |
| created_at | TEXT | NOT NULL | 创建时间 |
| updated_at | TEXT | NOT NULL | 更新时间 |

**loop 表结构：**

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PRIMARY KEY AUTOINCREMENT | 主键 |
| type | TEXT | NOT NULL | 任务类型：`once`（一次性）/ `periodic`（周期） |
| name | TEXT | NOT NULL | 短标识，如 `self-cognition` |
| describe | TEXT | NOT NULL | 简短描述，如 "自我认知" |
| content | TEXT | NOT NULL | 完整任务指令 |
| status | TEXT | NOT NULL | once: `pending` / `done`；periodic: 始终 `active` |
| executed_count | INTEGER | DEFAULT 0 | 执行次数计数器 |
| result | TEXT | DEFAULT '' | 执行结果，periodic 每次覆盖 |
| weight | REAL | DEFAULT 0.5 | 优先级权重 |
| created_at | TEXT | NOT NULL | 创建时间 |
| updated_at | TEXT | NOT NULL | 更新时间 |

### 3.2 记忆部分

| 表 | 说明 |
|----|------|
| memory | 记忆存储 |
| forget | 遗忘机制（待定义） |
| pages | 页面上下文 |

**memory 表结构：**

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PRIMARY KEY AUTOINCREMENT | 主键 |
| content | TEXT | NOT NULL | 记忆内容 |
| created_at | TEXT | NOT NULL | 创建时间 |
| importance | REAL | DEFAULT 0.5 | 重要度 0.0-1.0 |

**pages 表结构：**

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PRIMARY KEY AUTOINCREMENT | 主键 |
| page_id | TEXT | NOT NULL | 页面唯一 ID（UUID） |
| cwd | TEXT | | 工作目录 |
| context | TEXT | NOT NULL | 页面上下文（JSON 格式） |
| created_at | TEXT | NOT NULL | 创建时间 |
| updated_at | TEXT | NOT NULL | 更新时间 |

**模板页面：** `page_id='0'` 的记录存储默认上下文，新建页面时自动继承模板的 `context` 数据。

**context 字段格式：** 一个 JSON 对象，key 为自定义字段名，value 支持三种类型：

| 类型 | 格式 | 说明 | 示例 |
|------|------|------|------|
| 纯字符串 | `"key": "value"` | 直接存储字符串值 | `"system_prompt": "你是一个AI助手"` |
| 文件引用 | `"key": {"@file": "path"}` | 相对 iroll 包根目录的文件路径，读取时解析为文件内容 | `"greeting": {"@file": "Resources/greeting.txt"}` |
| SQL 查询 | `"key": {"@sql": "SELECT ..."}` | SQL 查询语句，读取时解析为查询结果 | `"description": {"@sql": "SELECT value FROM metadata WHERE key = 'description'"}` |

**解析规则：** `@file` 和 `@sql` 在读取时（`get-context`、`page new`）解析为实际值。写入时（`update-context`）存储原始标记，不做解析。

### 3.3 知识部分

| 表 | 说明 |
|----|------|
| book | 知识书籍（待定义，对应 Resources/books/ 目录） |
| skill | 技能（待定义，对应 Resources/skills/ 目录） |

### 3.4 其他部分

| 表 | 说明 |
|----|------|
| history | 构建历史（分层构建时自动维护） |

## 4. 工作目录 ~/.iroll/

所有 .iroll 包加载后解压到 `~/.iroll/` 目录下，以包名称为子目录：

```
~/.iroll/
├── system.db              # 全局系统数据库
├── base-agent/            # 已加载的 iroll 包
│   ├── Resources/
│   └── ai_roll.db
└── python-expert/
    ├── Resources/
    └── ai_roll.db
```

### 4.1 系统数据库 system.db

全局管理，不依赖任何单个 iroll 包：

| 表 | 说明 |
|----|------|
| page_index | 所有页面的索引（iroll_name, page_id, cwd） |
| active_page | 按工作目录追踪活跃页面（每个 cwd 一条记录） |
| config | 全局配置 key-value |

**page_index 表结构：**

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PRIMARY KEY AUTOINCREMENT | 主键 |
| iroll_name | TEXT | NOT NULL | 所属 iroll 包名 |
| page_id | TEXT | NOT NULL | 页面 ID |
| cwd | TEXT | NOT NULL | 工作目录 |
| created_at | TEXT | NOT NULL | 创建时间 |

**active_page 表结构：**

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PRIMARY KEY AUTOINCREMENT | 主键 |
| cwd | TEXT | NOT NULL UNIQUE | 工作目录（唯一） |
| iroll_name | TEXT | NOT NULL | 活跃页面所属 iroll 包 |
| page_id | TEXT | NOT NULL | 活跃页面 ID |
| updated_at | TEXT | NOT NULL | 更新时间 |

## 5. CLI 命令 logos

系统无需手动初始化，首次使用任何命令时会自动创建 `~/.iroll/` 和 `system.db`。

### 5.1 系统状态

| 命令 | 说明 |
|------|------|
| `logos status` | 查看系统状态（home 路径、iroll 数量、page 数量、roll 列表） |

### 5.2 包管理

| 命令 | 说明 |
|------|------|
| `logos roll build -f <file> -t <name>` | 从 Layerfile 分层构建 iroll |
| `logos roll load <file>` | 加载 .iroll 文件到 ~/.iroll/ |
| `logos roll list` | 列出所有已加载的 iroll |
| `logos roll rm <name>` | 删除一个 iroll 包（同时清理 system.db 中的相关记录） |
| `logos roll save <name> [-o path]` | 将 iroll 打包为 .iroll 文件 |
| `logos roll inspect <name>` | 查看 iroll 详情（metadata、表统计、资源列表） |
| `logos roll history <name>` | 查看构建历史 |

### 5.3 页面管理

| 命令 | 说明 |
|------|------|
| `logos page new <name> [--cwd .]` | 创建新页面（继承模板 page_id='0' 的 context，自动设为当前 cwd 的活跃页面） |
| `logos page current [--cwd .]` | 查看当前活跃页面 |
| `logos page list [name] [--cwd .] [-a]` | 列出页面。不指定 name 查全局索引，`-a` 查所有 cwd |
| `logos page switch <page-id>` | 切换活跃页面 |
| `logos page delete <page-id>` | 删除页面 |
| `logos page get-context [name] [--page <id>] [--cwd .]` | 获取上下文（返回解析后的实际值） |
| `logos page update-context [name] --content <json> [--page <id>] [--cwd .]` | 更新上下文（存储原始 JSON 标记） |
| `logos page add-memory [name] --content <text> [--importance 0.5] [--cwd .]` | 新增记忆 |

**省略模式：** `page new` 后自动设为活跃页面，后续命令可省略 name 和 --page，自动使用当前 cwd 的活跃页面。

### 5.4 分层构建

**Layerfile 指令（仅三条）：**

| 指令 | 格式 | 说明 |
|------|------|------|
| FROM | `FROM <name>` | 指定基础层（本地 ~/.iroll/ 下已有的包） |
| MIGRATE | `MIGRATE <file.sql>` | 执行 SQL（建表、改字段、插数据） |
| COPY | `COPY <src> <dest>` | 复制文件到 Resources/ |

## 6. 技术栈

- Go 1.24, Cobra CLI 框架
- SQLite（go-sqlite3, CGO）
- 纯标准库（无第三方运行时依赖）

## 7. 路线图

### 已完成

- [x] .iroll 包格式定义（ZIP + SQLite）
- [x] system.db 全局页面索引 + 按 cwd 追踪活跃页面
- [x] CLI 命令体系（status / roll / page 三大类）
- [x] context 标准化格式（纯字符串 / @file / @sql 三种值类型，读时解析）
- [x] 模板页面（page_id='0'）继承机制
- [x] 页面管理（new / current / list / switch / delete / get-context / update-context / add-memory）
- [x] 分层构建（FROM / MIGRATE / COPY）
- [x] 构建历史追踪
- [x] dna 表（决策 DNA：认知观/伦理观/审美观/本体观）
- [x] loop 表（循环任务：一次性/周期）

### 待做

- [ ] loop 命令行支持（list/finish/add）
- [ ] 遗忘表定义
- [ ] book 表 + Resources/books/ 知识检索
- [ ] skill 表 + Resources/skills/ 技能管理
- [ ] 记忆检索（按重要性/关键词查询）
- [ ] 基础信息获取完善
- [ ] engine（心跳）机制
- [ ] 前端界面
- [ ] 斜杠命令表

