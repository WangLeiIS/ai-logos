# Logos CLI irollhub 集成设计规格

## 概述

扩展 logos CLI，添加与 irollhub 注册中心的集成能力。用户可以通过 CLI 登录、搜索、推送和拉取 .iroll 包。

**目标：** 完成 .iroll 包的发现、下载和发布闭环，让好的 .iroll 能被创造和分享。

## 技术选型

| 决策 | 选择 | 原因 |
|------|------|------|
| HTTP 客户端 | 标准库 `net/http` | 无额外依赖，功能充足 |
| 配置格式 | INI | Git 风格，开发者熟悉，支持多 hub |
| 输出格式 | JSON | 与现有 CLI 命令一致 |
| 重试策略 | 指数退避（1s, 2s, 4s） | 网络请求最佳实践 |

## 命令设计

### 1. login

```bash
# 默认模式（使用默认 hub URL）
logos roll login

# 指定 hub URL
logos roll login --hub https://irollhub.example.com
```

**行为：**
1. 读取当前配置（环境变量 > 配置文件 > 默认值 `http://localhost:8080`）
2. 打开浏览器访问 `{hub}/api/v1/auth/github`
3. 提示用户：`Please enter your API Key (copied from browser):`
4. 验证 API Key（调用 `/api/v1/auth/me`）
5. 保存到 `~/.iroll/config`
6. 输出：`{"message": "Logged in to https://irollhub.example.com as wanglei"}`

**错误处理：**
- 网络错误：重试 3 次（指数退避），提示 `Retrying... (1/3)`
- 验证失败：`{"error": "Invalid API key", "code": "UNAUTHORIZED"}`

---

### 2. logout

```bash
logos roll logout
```

**行为：**
1. 删除 `~/.iroll/config` 中的 `[hub "..."]` 部分
2. 如果配置文件为空，删除整个文件
3. 输出：`{"message": "Logged out"}`

---

### 3. push

```bash
# 推送文件
logos roll push <file.iroll> <org>/<pkg>:<ver>

# 推送已加载的包
logos roll push <org>/<pkg>:<ver>
```

**行为：**
1. 读取 .iroll 文件（从路径或 `~/.iroll/<name>/` 重新打包）
2. 校验文件（ZIP 格式、包含 ai_roll.db）
3. 上传到 `{hub}/api/v1/orgs/{org}/packages/{pkg}/versions`（multipart/form-data）
4. 输出：`{"message": "Pushed official/cat-agent:v1.0.0", "size": 12345}`

**错误处理：**
- 文件不存在：`{"error": "file not found", "code": "NOT_FOUND"}`
- 校验失败：`{"error": "invalid iroll package", "code": "INVALID_PACKAGE"}`
- 冲突（版本已存在）：`{"error": "version already exists", "code": "CONFLICT"}`
- 网络错误：重试 3 次

---

### 4. pull

```bash
# 下载并加载（默认）
logos roll pull <org>/<pkg>:<ver>
logos roll pull <org>/<pkg>        # 默认为 latest

# 仅下载
logos roll pull <org>/<pkg>:<ver> -o <path>
```

**行为：**
1. 调用 `{hub}/.../download`（302 → MinIO 预签名 URL）
2. 下载文件
3. 如果没有 `-o`，调用 `load` 命令安装到 `~/.iroll/<name>/`
4. 输出：`{"message": "Pulled official/cat-agent:v1.0.0", "name": "cat-agent", "path": "~/.iroll/cat-agent"}`

**错误处理：**
- 未找到：`{"error": "package not found", "code": "NOT_FOUND"}`
- 网络错误：重试 3 次

---

### 5. search

```bash
logos roll search <keyword> [--tag <tag>]
```

**行为：**
1. 调用 `{hub}/api/v1/search?q={keyword}&tag={tag}`
2. 输出 JSON：`{"results": [{"org": "...", "package": "...", "version": "...", "description": "..."}]}`

**参数：**
- `keyword`：搜索关键词（必需）
- `--tag`：按标签过滤（可选）

## 配置管理

### 配置文件格式

**`~/.iroll/config`** （INI 格式）：

```ini
[hub "https://irollhub.example.com"]
    token = iroll_abc123...
```

**多 hub 支持（未来扩展）：**

```ini
[hub "https://irollhub.example.com"]
    token = iroll_abc123...

[hub "https://private-hub.company.com"]
    token = iroll_xyz789...
```

### 环境变量

| 环境变量 | 说明 | 优先级 |
|---------|------|--------|
| `IROLLHUB_URL` | Hub URL | 高于配置文件 |
| `IROLLHUB_TOKEN` | API Token | 高于配置文件 |

### 配置读取逻辑

```
1. 检查环境变量 IROLLHUB_URL、IROLLHUB_TOKEN
2. 如果没有，读取 ~/.iroll/config
3. 如果都没有，使用默认值（http://localhost:8080）
4. 对于需要认证的操作，如果没有任何 token，返回错误
```

### 配置写入逻辑

```
1. 打开 ~/.iroll/config（不存在则创建）
2. 如果已存在 [hub "..."] 部分，替换 token
3. 如果不存在，追加新的 [hub "..."] 部分
4. 设置文件权限为 0600（仅用户可读写）
```

## HTTP 客户端

### 客户端结构

在 `iroll/cmd/hub_config.go` 中创建 `HubClient` 结构：

```go
type HubClient struct {
    BaseURL string
    Token   string
    Client  *http.Client
}

func NewHubClient(url, token string) *HubClient
func (c *HubClient) Get(path string) (*http.Response, error)
func (c *HubClient) Post(path string, body io.Reader) (*http.Response, error)
func (c *HubClient) Download(path string) ([]byte, error)
```

### 请求处理

**认证：**
- 如果有 Token，添加 `Authorization: Bearer {token}` 头
- 读操作（search、download）无需认证
- 写操作（push）必须认证

**Content-Type：**
- JSON 请求：`application/json`
- 文件上传：`multipart/form-data`

### 重试逻辑

```go
func retry(fn func() error, maxAttempts int) error {
    delay := time.Second
    for attempt := 0; attempt < maxAttempts; attempt++ {
        err := fn()
        if err == nil {
            return nil
        }
        if !isNetworkError(err) {
            return err  // 非网络错误，立即失败
        }
        if attempt == maxAttempts-1 {
            return err  // 最后一次尝试失败
        }
        fmt.Fprintf(os.Stderr, "Retrying... (%d/%d)\n", attempt+1, maxAttempts)
        time.Sleep(delay)
        delay *= 2  // 指数退避
    }
    return nil
}
```

### 错误处理

**网络错误判断：**
- 超时、连接 refused、DNS 失败 → 网络错误，重试
- HTTP 4xx/5xx → 非网络错误，立即失败

**API 错误响应：**
解析 irollhub 返回的 JSON：`{"error": "...", "code": "..."}`，转换为 CLI 输出格式。

## 错误分类

| 错误类型 | HTTP 状态 | CLI 输出 | 重试 |
|---------|-----------|---------|------|
| 网络错误 | - | `{"error": "network error", "code": "NETWORK_ERROR"}` | 是 |
| 未认证 | 401 | `{"error": "not logged in", "code": "UNAUTHORIZED"}` | 否 |
| 未找到 | 404 | `{"error": "package not found", "code": "NOT_FOUND"}` | 否 |
| 冲突 | 409 | `{"error": "version already exists", "code": "CONFLICT"}` | 否 |
| 校验失败 | 400 | `{"error": "invalid package", "code": "INVALID"}` | 否 |
| 服务器错误 | 500 | `{"error": "server error", "code": "SERVER_ERROR"}` | 是 |

## 文件结构

```
iroll/cmd/
├── hub_login.go        # login/logout 命令 + 配置读写
├── hub_push.go         # push 命令
├── hub_pull.go         # pull 命令
└── hub_search.go       # search 命令
```

**每个文件的结构（以 hub_login.go 为例）：**

```go
package cmd

import (
    // ...
)

var loginCmd = &cobra.Command{
    Use:   "login [--hub <url>]",
    Short: "Login to irollhub",
    Run: func(cmd *cobra.Command, args []string) {
        // 1. 读取配置
        // 2. 打开浏览器
        // 3. 读取 API Key
        // 4. 验证
        // 5. 保存
        // 6. 输出成功
    },
}

var logoutCmd = &cobra.Command{
    Use:   "logout",
    Short: "Logout from irollhub",
    Run: func(cmd *cobra.Command, args []string) {
        // 删除配置
    },
}

func init() {
    rollCmd.AddCommand(loginCmd)
    rollCmd.AddCommand(logoutCmd)
}
```

## 测试策略

1. **单元测试** — 配置读写、错误判断、网络逻辑
2. **集成测试** — 使用测试 irollhub 实例
3. **手动测试** — 真实 hub 环境测试 OAuth 流程

## 不做什么

第一版不做：

- 多 hub 同时使用（只支持单一 hub 配置）
- 包的依赖关系解析
- 增量上传/断点续传
- 上传进度条（大文件上传）

## 刻意不做的功能

- **自动版本号生成** — 版本号必须由用户明确指定，保持与 Docker 一致
- **复杂的重试逻辑** — 只重试网络错误，不重试业务错误
- **表格输出格式** — 统一使用 JSON，便于脚本处理
