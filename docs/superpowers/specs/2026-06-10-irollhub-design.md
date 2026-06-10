# irollhub 设计规格

## 概述

irollhub 是 .iroll 包的注册中心，对标 Docker Hub / Harbor。提供托管、版本管理和分发能力。

三层结构：组织（= 用户）→ 包 → 版本。

## 技术选型

| 决策 | 选择 | 原因 |
|------|------|------|
| 模块 | 独立 Go module | 独立部署和版本管理，后续独立仓库 |
| HTTP 框架 | Gin | 生态成熟，开发速度快 |
| 元数据 | SQLite | 和 logos 风格一致，单实例足够 |
| 文件存储 | MinIO | 已有服务，支持预签名 URL |
| 搜索 | SQLite FTS5 | 内置全文搜索，支持模糊匹配和排名，零额外依赖 |
| API Key | `iroll_` 前缀 + 32 字节随机串 | 类 GitHub PAT 风格，易辨识 |

## 架构

```
logos CLI (push / pull / search / login)
    │ HTTP (Bearer token 认证)
    ▼
irollhub (Gin HTTP server)
    ├── hub.db (SQLite)
    │   ├── organizations / api_keys / packages / versions
    │   └── FTS5 虚拟表 (packages 全文索引)
    └── MinIO
        └── bucket: irollhub
            └── {org}/{package}/{version}.iroll
```

## 认证模型

**读操作无需认证**：列表、查看、下载、搜索。

**写操作需要 API Key**：创建组织/包、上传/删除版本。通过 `Authorization: Bearer iroll_xxx` 传递。

### OAuth 流程

1. 浏览器访问 `GET /api/v1/auth/github` → 302 跳转 GitHub 授权页
2. 用户授权 → GitHub 回调 `GET /api/v1/auth/github/callback?code=xxx`
3. irollhub 用 code 换 access_token → 获取用户信息 → 创建组织（首次）或匹配已有组织
4. 生成 API Key → 内联 HTML 页面显示 Key + 复制按钮
5. 用户将 Key 配置到 CLI：`logos roll login` → 输入 Key → 保存到 `~/.iroll/config`

Google OAuth 同理。

### API Key 规格

- 格式：`iroll_` + 64 个 hex 字符（32 字节 `crypto/rand`）
- 存储：SHA256 哈希，不存明文
- 验证：请求时计算输入 key 的 SHA256，比对数据库
- Key 只在创建时显示一次

## API 端点

### 认证（7 个）

```
GET    /api/v1/auth/github                发起 GitHub OAuth
GET    /api/v1/auth/github/callback       GitHub 回调
GET    /api/v1/auth/google                发起 Google OAuth
GET    /api/v1/auth/google/callback       Google 回调
GET    /api/v1/auth/me                    当前用户信息 [需认证]
POST   /api/v1/auth/keys                  生成新 API Key [需认证]
DELETE /api/v1/auth/keys/{key_id}         吊销 API Key [需认证]
```

### 组织（3 个）

```
GET    /api/v1/orgs                       列出组织
POST   /api/v1/orgs                       创建组织 [需认证]
GET    /api/v1/orgs/{org}                 查看组织（含包列表）
```

### 包（4 个）

```
GET    /api/v1/orgs/{org}/packages              列出包
POST   /api/v1/orgs/{org}/packages              创建包 [需认证，需为组织 owner]
GET    /api/v1/orgs/{org}/packages/{pkg}         查看包（含版本列表）
DELETE /api/v1/orgs/{org}/packages/{pkg}         删除包及其所有版本 [需认证，需为组织 owner]
```

### 版本（5 个）

```
GET    /api/v1/orgs/{org}/packages/{pkg}/versions                列出版本
POST   /api/v1/orgs/{org}/packages/{pkg}/versions                上传版本 [需认证，需为组织 owner]
GET    /api/v1/orgs/{org}/packages/{pkg}/versions/{ver}           查看版本详情
GET    /api/v1/orgs/{org}/packages/{pkg}/versions/{ver}/download  下载（302 → MinIO 预签名 URL）
DELETE /api/v1/orgs/{org}/packages/{pkg}/versions/{ver}           删除版本 [需认证，需为组织 owner]
```

### 搜索（1 个）

```
GET    /api/v1/search?q=keyword&tag=personality    搜索包（FTS5）
```

共 20 个端点。

## 数据模型

### organizations 表（= 用户）

注册即创建组织。用户名就是命名空间。

**组织名来源：** GitHub OAuth 使用 `login` 字段，Google OAuth 使用 email 的 `@` 前缀部分。如果名称已被占用，追加数字后缀（如 `wanglei2`）。

```sql
CREATE TABLE organizations (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL UNIQUE,
    provider    TEXT    NOT NULL,           -- 'github' | 'google'
    provider_id TEXT    NOT NULL,
    email       TEXT,
    avatar_url  TEXT,
    created_at  TEXT    NOT NULL
);
CREATE UNIQUE INDEX idx_orgs_provider ON organizations(provider, provider_id);
```

### api_keys 表

```sql
CREATE TABLE api_keys (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    org_id      INTEGER NOT NULL REFERENCES organizations(id),
    key_hash    TEXT    NOT NULL UNIQUE,    -- SHA256 of the raw key
    name        TEXT    NOT NULL DEFAULT 'default',
    last_used_at TEXT,
    created_at  TEXT    NOT NULL
);
CREATE INDEX idx_api_keys_org ON api_keys(org_id);
```

### packages 表

```sql
CREATE TABLE packages (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    org_id      INTEGER NOT NULL REFERENCES organizations(id),
    name        TEXT    NOT NULL,
    description TEXT    NOT NULL DEFAULT '',
    tags        TEXT    NOT NULL DEFAULT '[]',  -- JSON array
    downloads   INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT    NOT NULL,
    updated_at  TEXT    NOT NULL
);
CREATE UNIQUE INDEX idx_packages_org_name ON packages(org_id, name);
```

### versions 表

```sql
CREATE TABLE versions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    package_id  INTEGER NOT NULL REFERENCES packages(id),
    version     TEXT    NOT NULL,           -- semver, e.g. 'v1.0.0'
    object_key  TEXT    NOT NULL,           -- MinIO key: {org}/{pkg}/{ver}.iroll
    file_size   INTEGER NOT NULL,
    checksum    TEXT    NOT NULL,           -- SHA256 hex
    created_at  TEXT    NOT NULL
);
CREATE UNIQUE INDEX idx_versions_pkg_ver ON versions(package_id, version);
```

### FTS5 虚拟表

```sql
CREATE VIRTUAL TABLE packages_fts USING fts5(
    name,
    description,
    tags,
    content=packages,
    content_rowid=id
);
```

搜索时查询 `packages_fts`，JOIN `packages` 和 `organizations` 返回完整信息。

## 上传流程

```
POST multipart/form-data (file=@agent.iroll)
  │
  ├── 1. 认证检查 — Bearer token → 匹配 org → 检查 org owner 权限
  ├── 2. 大小检查 — ≤ 500MB
  ├── 3. ZIP 校验 — 合法 ZIP 归档
  ├── 4. 结构校验 — ZIP 内包含 ai_roll.db
  ├── 5. 版本冲突检查 — 同一 package 下 version 是否已存在 → 409
  ├── 6. 计算校验和 — SHA256
  ├── 7. 上传 MinIO — key = {org}/{pkg}/{ver}.iroll
  ├── 8. 写入 SQLite — versions 记录
  └── 9. 更新 FTS5 — packages_fts 同步
```

任何一步失败，不写入存储。MinIO 上传成功但 SQLite 写入失败时，需清理 MinIO 中的孤立文件。

**删除流程：** 删除包时，先删除 MinIO 中该包的所有版本文件，再删除 SQLite 中的 versions 和 packages 记录。删除版本时同理。

## 下载流程

```
GET .../versions/{ver}/download
  │
  ├── 1. 查询 SQLite → 获取 object_key
  ├── 2. packages.downloads += 1
  └── 3. 生成 MinIO 预签名 URL (GET, 5min TTL) → 302 重定向
```

服务不代理文件传输，全部由 MinIO 直接服务。

## 搜索实现

FTS5 索引覆盖包名、描述、标签。查询使用 BM25 排名：

```sql
SELECT p.*, o.name AS org_name,
       rank
FROM packages_fts f
JOIN packages p ON p.id = f.rowid
JOIN organizations o ON o.id = p.org_id
WHERE packages_fts MATCH ?
ORDER BY rank
LIMIT 20;
```

支持 `q`（全文搜索）和 `tag`（精确标签过滤）两个参数。

## 配置

```yaml
listen: ":8080"
db: "./hub.db"
minio:
  endpoint: "localhost:9000"
  access_key: "minioadmin"
  secret_key: "minioadmin"
  bucket: "irollhub"
  use_ssl: false
oauth:
  github_client_id: ""
  github_client_secret: ""
  google_client_id: ""
  google_client_secret: ""
  redirect_base: "http://localhost:8080"    # 用于生成回调 URL
```

敏感配置支持环境变量覆盖：`IROLLHUB_GITHUB_CLIENT_SECRET` 等。

## 错误响应格式

所有错误返回统一 JSON：

```json
{
  "error": "version v1.0.0 already exists",
  "code": "CONFLICT"
}
```

标准 HTTP 状态码：400（校验失败）、401（未认证）、403（无权限）、404（不存在）、409（冲突）、500（内部错误）。

## 项目结构

```
irollhub/
├── main.go              入口：加载配置、初始化 DB/MinIO、注册 Gin 路由、启动服务
├── config.go            配置加载（YAML + 环境变量覆盖）
├── handler/
│   ├── auth.go          OAuth 流程 + API Key 管理
│   ├── org.go           组织 CRUD
│   ├── package.go       包 CRUD
│   ├── version.go       版本上传/下载/删除
│   └── search.go        FTS5 搜索
├── store/
│   ├── db.go            SQLite 初始化 + 所有 SQL 操作
│   └── minio.go         MinIO 初始化 + 上传/下载/删除
├── model/
│   └── model.go         Organization / Package / Version / APIKey 结构体
├── middleware/
│   └── auth.go          Bearer token 认证中间件
├── config.yaml          默认配置
└── readme.md
```

## 刻意不做

第一版不做：

- 权限管理 — 只有组织 owner 能操作
- 前端页面 — OAuth 回调页只显示 API Key
- 依赖解析 — .iroll 包之间无依赖
- 构建触发 — 不关联 CI/CD
- 镜像代理 — 不做缓存和分发加速
- 同一邮箱多 Provider 合并 — GitHub 和 Google 注册为不同组织

## CLI 集成（logos CLI 侧）

irollhub 服务本身不包含 CLI 代码。logos CLI 需要新增以下命令：

```bash
logos roll login --hub <url>           # OAuth 浏览器登录 → 输入 API Key
logos roll push <org>/<pkg>:<ver>      # 上传 .iroll 到 hub
logos roll pull <org>/<pkg>[:<ver>]    # 从 hub 下载并加载 .iroll
logos roll search <keyword>            # 搜索 hub 上的包
```

凭证存储在 `~/.iroll/config`，支持环境变量 `IROLLHUB_URL` / `IROLLHUB_TOKEN`。

这部分在 logos CLI 的独立实现计划中完成，不在 irollhub 服务范围内。
