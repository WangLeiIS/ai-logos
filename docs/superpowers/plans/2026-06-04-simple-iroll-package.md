# 最简 .iroll 包实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用单个 Python 脚本生成一个最简 `.iroll` 包（ZIP 内含 SQLite 数据库 + Resources 目录）

**Architecture:** 单脚本 `create_iroll.py` 使用 Python 标准库 `sqlite3` + `zipfile` + `tempfile`，在临时目录中构建数据库和资源文件，然后打包为 `.iroll`

**Tech Stack:** Python 3.12（标准库 only），uv 虚拟环境

---

## File Structure

| 文件 | 操作 | 职责 |
|------|------|------|
| `create_iroll.py` | 创建 | 生成 .iroll 包的主脚本 |
| `output/hello.iroll` | 产出 | 生成的 .iroll 包文件 |

---

### Task 1: 创建生成脚本 create_iroll.py

**Files:**
- Create: `create_iroll.py`

- [ ] **Step 1: 创建 create_iroll.py 脚本文件**

```python
import sqlite3
import zipfile
import tempfile
import os
from datetime import datetime, timezone


def create_db(db_path: str) -> None:
    """创建 ai_roll.db 并建表、插入示例数据。"""
    conn = sqlite3.connect(db_path)
    cur = conn.cursor()

    cur.execute("""
        CREATE TABLE metadata (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            name TEXT NOT NULL,
            version TEXT NOT NULL,
            created_at TEXT NOT NULL,
            description TEXT
        )
    """)

    cur.execute("""
        CREATE TABLE memory (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            content TEXT NOT NULL,
            created_at TEXT NOT NULL,
            importance REAL DEFAULT 0.5
        )
    """)

    now = datetime.now(timezone.utc).isoformat()

    cur.execute(
        "INSERT INTO metadata (name, version, created_at, description) VALUES (?, ?, ?, ?)",
        ("hello-agent", "0.1.0", now, "一个最简的 .iroll 示例 agent"),
    )

    cur.execute(
        "INSERT INTO memory (content, created_at, importance) VALUES (?, ?, ?)",
        ("你好，我是 hello-agent", now, 0.8),
    )
    cur.execute(
        "INSERT INTO memory (content, created_at, importance) VALUES (?, ?, ?)",
        ("这是一条示例记忆", now, 0.5),
    )

    conn.commit()
    conn.close()


def create_iroll(output_path: str) -> str:
    """生成 .iroll 包，返回产出文件路径。"""
    with tempfile.TemporaryDirectory() as tmpdir:
        resources_dir = os.path.join(tmpdir, "Resources")
        os.makedirs(resources_dir)

        with open(os.path.join(resources_dir, "README.txt"), "w", encoding="utf-8") as f:
            f.write("hello-agent 资源目录\n")

        db_path = os.path.join(tmpdir, "ai_roll.db")
        create_db(db_path)

        os.makedirs(os.path.dirname(output_path), exist_ok=True)

        with zipfile.ZipFile(output_path, "w", zipfile.ZIP_DEFLATED) as zf:
            for root, dirs, files in os.walk(tmpdir):
                for file in files:
                    file_path = os.path.join(root, file)
                    arcname = os.path.relpath(file_path, tmpdir)
                    zf.write(file_path, arcname)

    return output_path


if __name__ == "__main__":
    output = os.path.join("output", "hello.iroll")
    create_iroll(output)
    print(f"✓ 已生成: {output}")
```

- [ ] **Step 2: 运行脚本生成 .iroll 包**

Run: `cd "d:/worklog/code/语料库/ai-roll-mini" && .venv/Scripts/python.exe create_iroll.py`
Expected: 输出 `✓ 已生成: output/hello.iroll`

- [ ] **Step 3: 验证 .iroll 包是有效的 ZIP 文件**

Run: `cd "d:/worklog/code/语料库/ai-roll-mini" && .venv/Scripts/python.exe -m zipfile -l output/hello.iroll`
Expected: 列出 `Resources/README.txt` 和 `ai_roll.db`

- [ ] **Step 4: 验证 SQLite 数据库内容**

Run:
```bash
cd "d:/worklog/code/语料库/ai-roll-mini" && .venv/Scripts/python.exe -c "
import sqlite3, zipfile, tempfile, os
zf = zipfile.ZipFile('output/hello.iroll')
tmpdir = tempfile.mkdtemp()
zf.extractall(tmpdir)
conn = sqlite3.connect(os.path.join(tmpdir, 'ai_roll.db'))
print('--- metadata ---')
for row in conn.execute('SELECT * FROM metadata'):
    print(row)
print('--- memory ---')
for row in conn.execute('SELECT * FROM memory'):
    print(row)
conn.close()
"
```
Expected:
- metadata 表输出 1 行，name 为 `hello-agent`
- memory 表输出 2 行

- [ ] **Step 5: Commit**

```bash
git add create_iroll.py output/hello.iroll
git commit -m "feat: add create_iroll.py to generate minimal .iroll package"
```
