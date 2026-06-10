# irollhub

AI Agent 的包注册中心。对标 Docker Hub / Harbor，为 .iroll 包提供托管、版本管理和分发。

## 三层结构

```
组织 (Organization)
  └── 包 (Package)
        └── 版本 (Version)
```

**示例：**

```
official/                    ← 组织
  ├── cat-agent              ← 包
  │     ├── v1.0.0           ← 版本
  │     └── v1.1.0
  ├── tutor
  │     └── v2.0.0
  └── code-reviewer
wanglei/
  ├── pm
  └── daily-assistant
```

### 组织（Organization）

包的归属单位。个人或团队都可以创建组织。

### 包（Package）

一个 .iroll 包的仓库。属于一个组织，包含描述、标签和所有历史版本。

### 版本（Version）

一个具体的 .iroll 文件。遵循语义化版本（semver）。每个版本对应一个不可变的 .iroll 文件。

## 寻址格式

```
org/package:version
```

示例：
- `official/cat-agent:latest` — 最新版
- `official/cat-agent:v1.0.0` — 指定版本
- `wanglei/pm:v0.1.0` — 个人发布的包

省略版本时默认为 `latest`。

## 架构

```
                    ┌──────────────┐
                    │  logos CLI   │  push / pull / search
                    └──────┬───────┘
                           │ HTTP
                    ┌──────▼───────┐
                    │  irollhub    │  Go HTTP 服务
                    │  web server  │
                    └──┬───────┬───┘
                       │       │
              ┌────────▼┐   ┌──▼────────┐
              │ hub.db  │   │   MinIO    │
              │ SQLite  │   │  对象存储   │
              │ 元数据   │   │ .iroll 文件│
              └─────────┘   └───────────┘
```

**irollhub** 是一个 Go HTTP 服务。元数据存 SQLite，.iroll 文件存 MinIO。无前端页面，纯 API 服务，通过 CLI 交互。

## API

最小化接口，只做必要的事。读操作（列表、查看、下载、搜索）无需认证。写操作（创建组织、上传包）需要 API Key。

### 认证

```
GET    /api/v1/auth/github              发起 GitHub OAuth 登录
GET    /api/v1/auth/github/callback     GitHub OAuth 回调，签发 API Key
GET    /api/v1/auth/google              发起 Google OAuth 登录
GET    /api/v1/auth/google/callback     Google OAuth 回调，签发 API Key
GET    /api/v1/auth/me                  查看当前用户信息（需 API Key）
POST   /api/v1/auth/keys                生成新的 API Key（需已登录）
DELETE /api/v1/auth/keys/{key_id}       吊销 API Key
```

**流程：**

1. 浏览器访问 `/api/v1/auth/github` → 跳转 GitHub 授权
2. GitHub 回调 → irollhub 创建用户（首次）或匹配已有用户 → 签发 API Key
3. 用户在页面拿到 API Key → 配置到 CLI

**CLI 登录流程：**

```bash
logos roll login --hub https://irollhub.example.com
# 输出：请在浏览器打开 https://irollhub.example.com/auth/github
# 登录后输入 API Key: xxxx
# 保存到 ~/.iroll/config
```

后续所有写操作通过 `Authorization: Bearer <api_key>` 认证。

### 组织

```
GET    /api/v1/orgs                    列出组织
POST   /api/v1/orgs                    创建组织
GET    /api/v1/orgs/{org}              查看组织
```

### 包

```
GET    /api/v1/orgs/{org}/packages              列出包
POST   /api/v1/orgs/{org}/packages              创建包
GET    /api/v1/orgs/{org}/packages/{pkg}         查看包（含版本列表）
DELETE /api/v1/orgs/{org}/packages/{pkg}         删除包
```

### 版本

```
GET    /api/v1/orgs/{org}/packages/{pkg}/versions              列出版本
POST   /api/v1/orgs/{org}/packages/{pkg}/versions              上传新版本（multipart: .iroll 文件，上传时校验）
GET    /api/v1/orgs/{org}/packages/{pkg}/versions/{ver}         查看版本详情
GET    /api/v1/orgs/{org}/packages/{pkg}/versions/{ver}/download  下载 .iroll 文件（302 重定向到 MinIO 预签名 URL）
DELETE /api/v1/orgs/{org}/packages/{pkg}/versions/{ver}         删除版本
```

### 搜索

```
GET    /api/v1/search?q=keyword&tag=personality    全文搜索包
```

## MinIO 存储结构

```
Bucket: irollhub
  ├── official/
  │   └── cat-agent/
  │       ├── v1.0.0.iroll
  │       └── v1.1.0.iroll
  ├── tutor/
  │   └── v2.0.0.iroll
  └── wanglei/
      └── pm/
          └── v0.1.0.iroll
```

Key 格式：`{org}/{package}/{version}.iroll`

上传时计算 SHA256 校验和，与元数据一同写入 SQLite。下载时生成 MinIO 预签名 URL（302 重定向），服务本身不代理文件传输。

### 上传校验

上传 .iroll 文件时执行以下校验，全部通过才写入 MinIO：

1. **大小限制** — 文件不超过 500MB
2. **ZIP 格式** — 必须是合法的 ZIP 归档
3. **结构校验** — ZIP 内必须包含 `ai_roll.db` 文件

校验失败返回 `400 Bad Request` 并说明原因，不写入存储。

## 数据模型

### organizations 表（= 用户）

用户就是组织。注册即创建组织，用户名就是命名空间。

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | 主键 |
| name | TEXT UNIQUE | 组织名（= 用户名） |
| provider | TEXT | OAuth 提供商：`github` / `google` |
| provider_id | TEXT | 提供商侧的用户 ID |
| email | TEXT | 邮箱 |
| avatar_url | TEXT | 头像 |
| created_at | TEXT | 注册时间 |

唯一约束：`(provider, provider_id)`

### api_keys 表

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | 主键 |
| org_id | INTEGER FK | 所属组织 |
| key_hash | TEXT | API Key 的 SHA256 哈希（不存明文） |
| name | TEXT | Key 名称（如 "my-laptop"） |
| last_used_at | TEXT | 最后使用时间 |
| created_at | TEXT | 创建时间 |

### packages 表

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | 主键 |
| org_id | INTEGER FK | 所属组织 |
| name | TEXT | 包名（org 内唯一） |
| description | TEXT | 包描述 |
| tags | TEXT | 标签 JSON 数组 |
| downloads | INTEGER | 下载量 |
| created_at | TEXT | 创建时间 |
| updated_at | TEXT | 更新时间 |

### versions 表

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | 主键 |
| package_id | INTEGER FK | 所属包 |
| version | TEXT | 语义化版本 |
| object_key | TEXT | MinIO 对象 Key |
| file_size | INTEGER | 文件大小（字节） |
| checksum | TEXT | SHA256 校验 |
| created_at | TEXT | 上传时间 |

## CLI 集成

扩展现有 `logos roll` 命令：

```bash
# 登录 hub
logos roll login --hub https://irollhub.example.com

# 推送到 hub（自动读取当前 hub 地址和凭证）
logos roll push official/cat-agent:v1.0.0

# 从 hub 拉取
logos roll pull official/cat-agent:v1.0.0
logos roll pull official/cat-agent              # 等同于 :latest

# 搜索
logos roll search "代码审查"
```

凭证存储在 `~/.iroll/config` 或环境变量 `IROLLHUB_URL` / `IROLLHUB_TOKEN`。

## 配置

irollhub 服务通过环境变量或配置文件启动：

```yaml
# config.yaml
listen: ":8080"
db: "./hub.db"
minio:
  endpoint: "localhost:9000"
  access_key: "minioadmin"
  secret_key: "minioadmin"
  bucket: "irollhub"
  use_ssl: false
```

## 项目结构

```
irollhub/
├── main.go              # 入口，加载配置、启动 HTTP 服务
├── config.go            # 配置加载
├── handler/             # HTTP handler
│   ├── org.go           # 组织 CRUD
│   ├── package.go       # 包 CRUD
│   ├── version.go       # 版本上传/下载/删除
│   └── search.go        # 搜索
├── store/               # 数据层
│   ├── db.go            # SQLite 操作
│   └── minio.go         # MinIO 操作
├── model/               # 数据模型
│   └── model.go         # Organization / Package / Version
├── config.yaml          # 默认配置
└── readme.md
```

## 不做什么

第一版刻意不做：

- 权限管理 — 只有组织 owner 能操作，后续加协作者
- 前端页面 — 纯 API + CLI，OAuth 回调页只显示 API Key
- 依赖解析 — .iroll 包之间暂无依赖
- 构建触发 — 不关联 CI/CD
- 镜像代理 — 不做缓存和分发加速
