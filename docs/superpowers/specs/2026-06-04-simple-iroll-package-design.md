# 最简 .iroll 包设计

## 概述

创建一个最简单的 `.iroll` 包，包含元数据表（必须）和记忆表，使用单 Python 脚本生成。

## 产出文件

```
ai-roll-mini/
├── create_iroll.py       # 生成脚本
└── output/
    └── hello.iroll       # 产出 .iroll 包
```

## .iroll 包内部结构

```
hello.iroll (ZIP)
├── Resources/
│   └── README.txt        # 空资源占位文件
└── ai_roll.db            # SQLite 数据库
```

## 数据库表设计

### 元数据表 `metadata`（必须）

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PRIMARY KEY AUTOINCREMENT | 主键 |
| name | TEXT | NOT NULL | agent 名称 |
| version | TEXT | NOT NULL | 版本号 |
| created_at | TEXT | NOT NULL | 创建时间 ISO 8601 |
| description | TEXT | | agent 描述 |

### 记忆表 `memory`

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PRIMARY KEY AUTOINCREMENT | 主键 |
| content | TEXT | NOT NULL | 记忆内容 |
| created_at | TEXT | NOT NULL | 创建时间 ISO 8601 |
| importance | REAL | DEFAULT 0.5 | 重要度 0.0-1.0 |

## 生成脚本逻辑

1. 创建临时目录
2. 创建 `Resources/` 子目录，写入空占位文件 `README.txt`
3. 用 `sqlite3` 标准库创建 `ai_roll.db`，建 `metadata` 和 `memory` 两张表
4. 插入一条示例元数据和两条示例记忆
5. 用 `zipfile` 标准库将临时目录打包为 `output/hello.iroll`
6. 清理临时目录

## 约束

- 仅使用 Python 标准库（`sqlite3`、`zipfile`、`os`、`tempfile`）
- 无第三方依赖
- 使用 uv 虚拟环境运行
