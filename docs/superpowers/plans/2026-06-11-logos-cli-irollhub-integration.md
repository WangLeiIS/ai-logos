# Logos CLI irollhub 集成实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 扩展 logos CLI，添加与 irollhub 注册中心的集成能力（login、logout、push、pull、search 命令）

**Architecture:** 在 iroll/cmd/ 下新增四个文件（hub_login.go、hub_push.go、hub_pull.go、hub_search.go），使用标准库 net/http 与 irollhub API 通信。配置存储在 ~/.iroll/config（INI 格式），支持环境变量覆盖。

**Tech Stack:** Go 1.23+, Cobra CLI, net/http, ini（go-ini 库用于解析 INI 格式）

---

### Task 1: 创建 HTTP 客户端和配置管理

**Files:**
- Create: `iroll/cmd/hub_config.go`
- Modify: `iroll/go.mod`

- [ ] **Step 1: 添加 go-ini 依赖**

Run: `cd iroll && go get github.com/go-ini/ini`
Expected: 依赖添加到 go.mod 和 go.sum

- [ ] **Step 2: 创建 hub_config.go 文件**

```go
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/go-ini/ini"
)

const (
	DefaultHubURL     = "http://localhost:8080"
	ConfigFile        = ".iroll/config"
	MaxRetries        = 3
	InitialRetryDelay = time.Second
)

type HubConfig struct {
	HubURL string
	Token  string
}

// HubClient 封装了与 irollhub 的 HTTP 通信
type HubClient struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

// NewHubClient 创建一个新的 HubClient
func NewHubClient(url, token string) *HubClient {
	if url == "" {
		url = DefaultHubURL
	}
	// 确保 URL 不以斜杠结尾
	url = strings.TrimSuffix(url, "/")
	
	return &HubClient{
		BaseURL: url,
		Token:   token,
		Client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Get 发送 GET 请求
func (c *HubClient) Get(path string) (*http.Response, error) {
	req, err := http.NewRequest("GET", c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	
	return c.Client.Do(req)
}

// Post 发送 POST 请求
func (c *HubClient) Post(path string, body io.Reader, contentType string) (*http.Response, error) {
	req, err := http.NewRequest("POST", c.BaseURL+path, body)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Content-Type", contentType)
	
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	
	return c.Client.Do(req)
}

// Download 下载文件内容
func (c *HubClient) Download(path string) ([]byte, *http.Response, error) {
	resp, err := c.Get(path)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	
	return data, resp, nil
}

// isNetworkError 判断是否为网络错误
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	// 检查是否为网络相关的错误
	// 超时、连接拒绝、DNS 失败等
	errStr := err.Error()
	networkErrors := []string{
		"timeout",
		"connection refused",
		"no such host",
		"connection reset",
		"eof",
	}
	for _, msg := range networkErrors {
		if strings.Contains(strings.ToLower(errStr), msg) {
			return true
		}
	}
	return false
}

// retry 重试函数，仅对网络错误进行指数退避重试
func retry(fn func() error, maxAttempts int) error {
	delay := InitialRetryDelay
	for attempt := 0; attempt < maxAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		if !isNetworkError(err) {
			return err
		}
		if attempt == maxAttempts-1 {
			return err
		}
		fmt.Fprintf(os.Stderr, "Retrying... (%d/%d)\n", attempt+1, maxAttempts)
		time.Sleep(delay)
		delay *= 2
	}
	return nil
}

// homeDir 返回用户主目录
func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// configPath 返回配置文件路径
func configPath() string {
	return filepath.Join(homeDir(), ConfigFile)
}

// readConfig 读取配置
func readConfig() (*HubConfig, error) {
	// 优先检查环境变量
	hubURL := os.Getenv("IROLLHUB_URL")
	token := os.Getenv("IROLLHUB_TOKEN")
	
	if hubURL != "" || token != "" {
		if hubURL == "" {
			hubURL = DefaultHubURL
		}
		return &HubConfig{HubURL: hubURL, Token: token}, nil
	}
	
	// 读取配置文件
	cfg, err := ini.Load(configPath())
	if err != nil {
		if os.IsNotExist(err) {
			// 配置文件不存在，返回默认配置
			return &HubConfig{HubURL: DefaultHubURL, Token: ""}, nil
		}
		return nil, err
	}
	
	// 读取第一个 hub 配置
	var configHubURL, configToken string
	for _, section := range cfg.Sections() {
		if strings.HasPrefix(section.Name(), "hub ") {
			hubName := strings.Trim(section.Name(), "hub \"")
			hubName = strings.TrimSuffix(hubName, "\"")
			configHubURL = hubName
			configToken = section.Key("token").String()
			break
		}
	}
	
	if configHubURL == "" {
		configHubURL = DefaultHubURL
	}
	
	return &HubConfig{HubURL: configHubURL, Token: configToken}, nil
}

// writeConfig 写入配置
func writeConfig(hubURL, token string) error {
	configFile := configPath()
	
	// 确保目录存在
	configDir := filepath.Dir(configFile)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}
	
	var cfg *ini.File
	var err error
	
	// 如果配置文件存在，先读取
	if _, err := os.Stat(configFile); err == nil {
		cfg, err = ini.Load(configFile)
		if err != nil {
			return err
		}
	} else {
		cfg = ini.Empty()
	}
	
	// 查找或创建 hub section
	sectionName := fmt.Sprintf("hub \"%s\"", hubURL)
	section, err := cfg.GetSection(sectionName)
	if err != nil {
		section, err = cfg.NewSection(sectionName)
		if err != nil {
			return err
		}
	}
	
	if token == "" {
		// 删除 token
		section.DeleteKey("token")
		if len(section.KeyStrings()) == 0 {
			cfg.DeleteSection(sectionName)
		}
	} else {
		// 设置 token
		if _, err := section.NewKey("token", token); err != nil {
			return err
		}
	}
	
	// 写入文件
	if err := cfg.SaveTo(configFile); err != nil {
		return err
	}
	
	// 设置文件权限为 0600
	return os.Chmod(configFile, 0600)
}

// openBrowser 在浏览器中打开 URL
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default: // linux, etc.
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

// parseAPIError 解析 API 错误响应
func parseAPIError(resp *http.Response) error {
	var apiErr struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}
	
	if len(body) == 0 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}
	
	if err := json.Unmarshal(body, &apiErr); err != nil {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}
	
	if apiErr.Error == "" {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}
	
	return fmt.Errorf("%s (code: %s)", apiErr.Error, apiErr.Code)
}
```

- [ ] **Step 3: 验证代码编译**

Run: `cd iroll && go build -o /tmp/logos .`
Expected: 编译成功，无错误

- [ ] **Step 4: 提交**

Run: `git add iroll/cmd/hub_config.go iroll/go.mod iroll/go.sum && git commit -m "feat: add hub client and config management"`

---

### Task 2: 实现 login 命令

**Files:**
- Create: `iroll/cmd/hub_login.go`

- [ ] **Step 1: 创建 hub_login.go 文件**

```go
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

var (
	loginHubURL string
)

var loginCmd = &cobra.Command{
	Use:   "login [--hub <url>]",
	Short: "Login to irollhub",
	Run: func(cmd *cobra.Command, args []string) {
		// 读取配置
		config, err := readConfig()
		if err != nil {
			outputError(fmt.Sprintf("Failed to read config: %v", err))
		}
		
		// 命令行参数优先级最高
		hubURL := loginHubURL
		if hubURL == "" {
			hubURL = config.HubURL
		}
		
		// 构造 OAuth URL
		authURL := fmt.Sprintf("%s/api/v1/auth/github", hubURL)
		
		// 打开浏览器
		fmt.Println("Opening browser for OAuth authorization...")
		if err := openBrowser(authURL); err != nil {
			fmt.Printf("Could not open browser: %v\n", err)
			fmt.Printf("Please visit: %s\n", authURL)
		} else {
			fmt.Println("Browser opened. Complete authorization in the browser.")
		}
		
		// 提示用户输入 API Key
		fmt.Print("Please enter your API Key (copied from browser): ")
		reader := bufio.NewReader(os.Stdin)
		token, err := reader.ReadString('\n')
		if err != nil {
			outputError(fmt.Sprintf("Failed to read input: %v", err))
		}
		
		token = strings.TrimSpace(token)
		if token == "" {
			outputError("API Key cannot be empty")
		}
		
		// 验证 API Key
		client := NewHubClient(hubURL, token)
		
		var resp *http.Response
		err = retry(func() error {
			var err error
			resp, err = client.Get("/api/v1/auth/me")
			return err
		}, MaxRetries)
		
		if err != nil {
			outputError(fmt.Sprintf("Network error: %v", err))
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK {
			apiErr := parseAPIError(resp)
			outputError(fmt.Sprintf("Authentication failed: %v", apiErr))
		}
		
		// 解析响应获取用户信息
		var userInfo struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
			outputError(fmt.Sprintf("Failed to parse response: %v", err))
		}
		
		// 保存配置
		if err := writeConfig(hubURL, token); err != nil {
			outputError(fmt.Sprintf("Failed to save config: %v", err))
		}
		
		outputJSON(map[string]string{
			"message": fmt.Sprintf("Logged in to %s as %s", hubURL, userInfo.Name),
		})
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Logout from irollhub",
	Run: func(cmd *cobra.Command, args []string) {
		// 读取配置
		config, err := readConfig()
		if err != nil {
			outputError(fmt.Sprintf("Failed to read config: %v", err))
		}
		
		if config.Token == "" {
			outputError("Not logged in")
		}
		
		// 删除 token（保留 hub URL）
		if err := writeConfig(config.HubURL, ""); err != nil {
			outputError(fmt.Sprintf("Failed to update config: %v", err))
		}
		
		outputJSON(map[string]string{
			"message": "Logged out",
		})
	},
}

func init() {
	loginCmd.Flags().StringVar(&loginHubURL, "hub", "", "Hub URL")
	rollCmd.AddCommand(loginCmd)
	rollCmd.AddCommand(logoutCmd)
}
```

- [ ] **Step 2: 验证代码编译**

Run: `cd iroll && go build -o /tmp/logos .`
Expected: 编译成功

- [ ] **Step 3: 测试命令帮助**

Run: `/tmp/logos roll login --help`
Expected: 显示 login 命令帮助信息

- [ ] **Step 4: 提交**

Run: `git add iroll/cmd/hub_login.go && git commit -m "feat: add login/logout commands"`

---

### Task 3: 实现 search 命令

**Files:**
- Create: `iroll/cmd/hub_search.go`

- [ ] **Step 1: 创建 hub_search.go 文件**

```go
package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

var (
	searchTag string
)

var searchCmd = &cobra.Command{
	Use:   "search <keyword> [--tag <tag>]",
	Short: "Search packages on irollhub",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		keyword := args[0]
		
		// 读取配置
		config, err := readConfig()
		if err != nil {
			outputError(fmt.Sprintf("Failed to read config: %v", err))
		}
		
		client := NewHubClient(config.HubURL, "")
		
		// 构造查询 URL
		query := fmt.Sprintf("?q=%s", url.QueryEscape(keyword))
		if searchTag != "" {
			query += fmt.Sprintf("&tag=%s", url.QueryEscape(searchTag))
		}
		
		// 发送请求
		var resp *http.Response
		err = retry(func() error {
			var err error
			resp, err = client.Get("/api/v1/search" + query)
			return err
		}, MaxRetries)
		
		if err != nil {
			outputError(fmt.Sprintf("Network error: %v", err))
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK {
			apiErr := parseAPIError(resp)
			outputError(fmt.Sprintf("Search failed: %v", apiErr))
		}
		
		// 解析响应
		var result struct {
			Results []struct {
				Org        string `json:"org"`
				Package    string `json:"package"`
				Version    string `json:"version"`
				Downloads  int    `json:"downloads"`
				OrgName    string `json:"org_name"`
				PkgName    string `json:"pkg_name"`
				PkgDesc    string `json:"pkg_desc"`
			} `json:"results"`
		}
		
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			outputError(fmt.Sprintf("Failed to parse response: %v", err))
		}
		
		// 格式化输出
		type SearchResult struct {
			Org        string `json:"org"`
			Package    string `json:"package"`
			Version    string `json:"version"`
			Downloads  int    `json:"downloads"`
			OrgName    string `json:"org_name"`
			PkgName    string `json:"pkg_name"`
			PkgDesc    string `json:"pkg_desc"`
		}
		
		outputs := make([]SearchResult, len(result.Results))
		for i, r := range result.Results {
			outputs[i] = SearchResult{
				Org:       r.Org,
				Package:   r.Package,
				Version:   r.Version,
				Downloads: r.Downloads,
				OrgName:   r.OrgName,
				PkgName:   r.PkgName,
				PkgDesc:   r.PkgDesc,
			}
		}
		
		outputJSON(map[string]interface{}{
			"results": outputs,
		})
	},
}

func init() {
	searchCmd.Flags().StringVar(&searchTag, "tag", "", "Filter by tag")
	rollCmd.AddCommand(searchCmd)
}
```

- [ ] **Step 2: 验证代码编译**

Run: `cd iroll && go build -o /tmp/logos .`
Expected: 编译成功

- [ ] **Step 3: 测试命令帮助**

Run: `/tmp/logos roll search --help`
Expected: 显示 search 命令帮助信息

- [ ] **Step 4: 提交**

Run: `git add iroll/cmd/hub_search.go && git commit -m "feat: add search command"`

---

### Task 4: 实现 pull 命令

**Files:**
- Create: `iroll/cmd/hub_pull.go`

- [ ] **Step 1: 创建 hub_pull.go 文件**

```go
package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

var (
	pullOutput string
)

var pullCmd = &cobra.Command{
	Use:   "pull <org>/<pkg>[:<ver>] [-o <path>]",
	Short: "Pull a package from irollhub",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ref := args[0]
		
		// 解析引用
		// 格式: org/pkg:ver 或 org/pkg
		parts := strings.Split(ref, ":")
		if len(parts) > 2 {
			outputError("Invalid package reference. Use: org/pkg or org/pkg:version")
		}
		
		orgPkg := parts[0]
		version := "latest"
		if len(parts) == 2 && parts[1] != "" {
			version = parts[1]
		}
		
		orgAndPkg := strings.Split(orgPkg, "/")
		if len(orgAndPkg) != 2 {
			outputError("Invalid package reference. Use: org/pkg or org/pkg:version")
		}
		
		org := orgAndPkg[0]
		pkg := orgAndPkg[1]
		
		// 读取配置
		config, err := readConfig()
		if err != nil {
			outputError(fmt.Sprintf("Failed to read config: %v", err))
		}
		
		client := NewHubClient(config.HubURL, "")
		
		// 构造下载 URL
		downloadPath := fmt.Sprintf("/api/v1/orgs/%s/packages/%s/versions/%s/download", org, pkg, version)
		
		// 下载文件
		var data []byte
		var resp *http.Response
		err = retry(func() error {
			var err error
			data, resp, err = client.Download(downloadPath)
			return err
		}, MaxRetries)
		
		if err != nil {
			outputError(fmt.Sprintf("Network error: %v", err))
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK {
			apiErr := parseAPIError(resp)
			outputError(fmt.Sprintf("Download failed: %v", apiErr))
		}
		
		// 处理输出
		if pullOutput != "" {
			// 写入指定路径
			if err := os.WriteFile(pullOutput, data, 0644); err != nil {
				outputError(fmt.Sprintf("Failed to write file: %v", err))
			}
			
			outputJSON(map[string]string{
				"message": "Downloaded",
				"path":    pullOutput,
				"size":    fmt.Sprintf("%d", len(data)),
			})
		} else {
			// 保存到临时文件，然后加载
			tmpDir := os.TempDir()
			tmpFile := filepath.Join(tmpDir, fmt.Sprintf("%s-%s-%s.iroll", org, pkg, version))
			
			if err := os.WriteFile(tmpFile, data, 0644); err != nil {
				outputError(fmt.Sprintf("Failed to write temp file: %v", err))
			}
			defer os.Remove(tmpFile)
			
			// 读取包名
			name, err := store.ReadName(tmpFile)
			if err != nil {
				outputError(fmt.Sprintf("Failed to read package name: %v", err))
			}
			
			// 提取包
			if err := store.Extract(tmpFile, name); err != nil {
				outputError(fmt.Sprintf("Failed to extract package: %v", err))
			}
			
			outputJSON(map[string]string{
				"message": fmt.Sprintf("Pulled %s/%s:%s", org, pkg, version),
				"name":    name,
				"path":    checkedIrollPath(name),
			})
		}
	},
}

func init() {
	pullCmd.Flags().StringVarP(&pullOutput, "output", "o", "", "Output file path (default: load to ~/.iroll/)")
	rollCmd.AddCommand(pullCmd)
}
```

- [ ] **Step 2: 验证代码编译**

Run: `cd iroll && go build -o /tmp/logos .`
Expected: 编译成功

- [ ] **Step 3: 测试命令帮助**

Run: `/tmp/logos roll pull --help`
Expected: 显示 pull 命令帮助信息

- [ ] **Step 4: 提交**

Run: `git add iroll/cmd/hub_pull.go && git commit -m "feat: add pull command"`

---

### Task 5: 实现 push 命令

**Files:**
- Create: `iroll/cmd/hub_push.go`

- [ ] **Step 1: 创建 hub_push.go 文件**

```go
package cmd

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"logos/safepath"
	"logos/store"
)

var pushCmd = &cobra.Command{
	Use:   "push <file.iroll|name> <org>/<pkg>:<ver>",
	Short: "Push a package to irollhub",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		source := args[0]
		target := args[1]
		
		// 解析目标引用
		// 格式: org/pkg:ver
		parts := strings.Split(target, ":")
		if len(parts) != 2 {
			outputError("Invalid target. Use: org/pkg:version")
		}
		
		orgPkg := parts[0]
		version := parts[1]
		
		if version == "" {
			outputError("Version cannot be empty")
		}
		
		orgAndPkg := strings.Split(orgPkg, "/")
		if len(orgAndPkg) != 2 {
			outputError("Invalid target. Use: org/pkg:version")
		}
		
		org := orgAndPkg[0]
		pkg := orgAndPkg[1]
		
		// 验证 org 和 pkg 名称
		if err := safepath.ValidateName(org); err != nil {
			outputError(fmt.Sprintf("Invalid org name: %v", err))
		}
		if err := safepath.ValidateName(pkg); err != nil {
			outputError(fmt.Sprintf("Invalid package name: %v", err))
		}
		
		// 读取源文件
		var zipPath string
		if strings.HasSuffix(source, ".iroll") {
			// 文件路径
			zipPath = source
			if _, err := os.Stat(zipPath); err != nil {
				outputError(fmt.Sprintf("File not found: %s", zipPath))
			}
		} else {
			// 已加载的包名，需要重新打包
			irollPath, err := store.IrollPath(source)
			if err != nil {
				outputError(fmt.Sprintf("Invalid package name: %v", err))
			}
			
			if _, err := os.Stat(irollPath); err != nil {
				outputError(fmt.Sprintf("Package not loaded: %s", source))
			}
			
			// 临时打包
			tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("%s.iroll", source))
			if err := createIrollZip(irollPath, tmpFile); err != nil {
				outputError(fmt.Sprintf("Failed to create package: %v", err))
			}
			defer os.Remove(tmpFile)
			zipPath = tmpFile
		}
		
		// 校验 ZIP 文件
		if err := validateIrollZip(zipPath); err != nil {
			outputError(fmt.Sprintf("Invalid package: %v", err))
		}
		
		// 计算校验和
		checksum, fileSize, err := computeChecksum(zipPath)
		if err != nil {
			outputError(fmt.Sprintf("Failed to compute checksum: %v", err))
		}
		
		// 读取配置
		config, err := readConfig()
		if err != nil {
			outputError(fmt.Sprintf("Failed to read config: %v", err))
		}
		
		if config.Token == "" {
			outputError("Not logged in. Use 'logos roll login' first")
		}
		
		client := NewHubClient(config.HubURL, config.Token)
		
		// 打开文件
		file, err := os.Open(zipPath)
		if err != nil {
			outputError(fmt.Sprintf("Failed to open file: %v", err))
		}
		defer file.Close()
		
		// 创建 multipart form
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		
		part, err := writer.CreateFormFile("file", filepath.Base(zipPath))
		if err != nil {
			outputError(fmt.Sprintf("Failed to create form: %v", err))
		}
		
		if _, err := io.Copy(part, file); err != nil {
			outputError(fmt.Sprintf("Failed to write form: %v", err))
		}
		
		writer.Close()
		
		// 发送请求
		uploadPath := fmt.Sprintf("/api/v1/orgs/%s/packages/%s/versions?version=%s", org, pkg, version)
		
		var resp *http.Response
		err = retry(func() error {
			var err error
			resp, err = client.Post(uploadPath, body, writer.FormDataContentType())
			return err
		}, MaxRetries)
		
		if err != nil {
			outputError(fmt.Sprintf("Network error: %v", err))
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			apiErr := parseAPIError(resp)
			outputError(fmt.Sprintf("Upload failed: %v", apiErr))
		}
		
		// 解析响应
		var result struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			// 忽略解析错误，上传成功即可
		}
		
		outputJSON(map[string]interface{}{
			"message": fmt.Sprintf("Pushed %s/%s:%s", org, pkg, version),
			"size":    fileSize,
			"checksum": checksum,
		})
	},
}

// validateIrollZip 校验 .iroll 文件
func validateIrollZip(zipPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("not a valid zip file: %w", err)
	}
	defer r.Close()
	
	hasDB := false
	for _, f := range r.File {
		if f.Name == "ai_roll.db" {
			hasDB = true
			break
		}
	}
	
	if !hasDB {
		return fmt.Errorf("ai_roll.db not found in package")
	}
	
	return nil
}

// computeChecksum 计算 SHA256 校验和
func computeChecksum(zipPath string) (string, int64, error) {
	file, err := os.Open(zipPath)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	
	stat, err := file.Stat()
	if err != nil {
		return "", 0, err
	}
	
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", 0, err
	}
	
	return hex.EncodeToString(hash.Sum(nil)), stat.Size(), nil
}

// createIrollZip 从已加载的包创建 .iroll 文件
func createIrollZip(sourcePath, destPath string) error {
	// 创建目标文件
	destFile, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer destFile.Close()
	
	// 创建 ZIP writer
	zipWriter := zip.NewWriter(destFile)
	defer zipWriter.Close()
	
	// 遍历源目录
	return filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// 计算相对路径
		relPath, err := filepath.Rel(sourcePath, path)
		if err != nil {
			return err
		}
		
		if info.IsDir() {
			return nil
		}
		
		// 打开源文件
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()
		
		// 在 ZIP 中创建文件
		dest, err := zipWriter.Create(relPath)
		if err != nil {
			return err
		}
		
		// 复制内容
		_, err = io.Copy(dest, srcFile)
		return err
	})
}

func init() {
	rollCmd.AddCommand(pushCmd)
}
```

- [ ] **Step 2: 验证代码编译**

Run: `cd iroll && go build -o /tmp/logos .`
Expected: 编译成功

- [ ] **Step 3: 测试命令帮助**

Run: `/tmp/logos roll push --help`
Expected: 显示 push 命令帮助信息

- [ ] **Step 4: 提交**

Run: `git add iroll/cmd/hub_push.go && git commit -m "feat: add push command"`

---

### Task 6: 添加集成测试

**Files:**
- Create: `iroll/cmd/hub_integration_test.go`

- [ ] **Step 1: 创建集成测试文件**

```go
package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigReadWrite(t *testing.T) {
	// 临时配置目录
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config")
	
	// 模拟 home 目录
	originalHome := homeDir()
	defer func() {
		// 恢复配置
		_ = os.Remove(configPath())
	}()
	
	// 测试写入配置
	hubURL := "https://test.example.com"
	token := "test_token_123"
	
	err := writeConfig(hubURL, token)
	if err != nil {
		t.Fatalf("writeConfig failed: %v", err)
	}
	
	// 验证文件存在
	if _, err := os.Stat(configPath()); err != nil {
		t.Fatalf("config file not created: %v", err)
	}
	
	// 测试读取配置
	config, err := readConfig()
	if err != nil {
		t.Fatalf("readConfig failed: %v", err)
	}
	
	if config.HubURL != hubURL {
		t.Errorf("HubURL mismatch: got %s, want %s", config.HubURL, hubURL)
	}
	
	if config.Token != token {
		t.Errorf("Token mismatch: got %s, want %s", config.Token, token)
	}
	
	// 测试删除 token
	err = writeConfig(hubURL, "")
	if err != nil {
		t.Fatalf("writeConfig (delete) failed: %v", err)
	}
	
	config, err = readConfig()
	if err != nil {
		t.Fatalf("readConfig (after delete) failed: %v", err)
	}
	
	if config.Token != "" {
		t.Errorf("Token not deleted: got %s, want empty", config.Token)
	}
}

func TestIsNetworkError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantNet bool
	}{
		{"timeout error", &timeoutError{}, true},
		{"connection refused", &connRefusedError{}, true},
		{"no such host", &dnsError{}, true},
		{"non-network error", &os.PathError{}, false},
		{"nil error", nil, false},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNetworkError(tt.err)
			if got != tt.wantNet {
				t.Errorf("isNetworkError() = %v, want %v", got, tt.wantNet)
			}
		})
	}
}

// 辅助错误类型
type timeoutError struct{}
func (e *timeoutError) Error() string { return "timeout" }
func (e *timeoutError) Timeout() bool   { return true }

type connRefusedError struct{}
func (e *connRefusedError) Error() string { return "connection refused" }

type dnsError struct{}
func (e *dnsError) Error() string { return "no such host" }

func TestValidateIrollZip(t *testing.T) {
	// 使用真实的测试 .iroll 文件
	// 这里需要创建一个测试用的 .iroll 文件
	
	tmpDir := t.TempDir()
	testZip := filepath.Join(tmpDir, "test.iroll")
	
	// 创建有效的测试 ZIP（包含 ai_roll.db）
	err := createTestIrollZip(testZip, true)
	if err != nil {
		t.Fatalf("createTestIrollZip failed: %v", err)
	}
	
	err = validateIrollZip(testZip)
	if err != nil {
		t.Errorf("validateIrollZip (valid) failed: %v", err)
	}
	
	// 创建无效的测试 ZIP（不包含 ai_roll.db）
	invalidZip := filepath.Join(tmpDir, "invalid.iroll")
	err = createTestIrollZip(invalidZip, false)
	if err != nil {
		t.Fatalf("createTestIrollZip (invalid) failed: %v", err)
	}
	
	err = validateIrollZip(invalidZip)
	if err == nil {
		t.Error("validateIrollZip (invalid) should fail")
	}
}

func createTestIrollZip(path string, includeDB bool) error {
	// 创建测试用的 ZIP 文件
	// 使用 archive/zip 创建简单结构
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	
	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()
	
	if includeDB {
		// 创建一个空的 ai_roll.db 文件
		w, err := zipWriter.Create("ai_roll.db")
		if err != nil {
			return err
		}
		// 写入空内容（真实测试中应该写入有效的 SQLite 数据）
		w.Write([]byte(""))
	}
	
	return nil
}
```

- [ ] **Step 2: 运行测试**

Run: `cd iroll && go test ./cmd/... -v`
Expected: 所有测试通过

- [ ] **Step 3: 提交**

Run: `git add iroll/cmd/hub_integration_test.go && git commit -m "test: add hub integration tests"`

---

### Task 7: 最终验证和文档更新

**Files:**
- Modify: `iroll/readme.md`

- [ ] **Step 1: 验证所有命令**

Run: `cd iroll && go build -o /tmp/logos . && /tmp/logos roll --help`
Expected: 显示所有子命令，包括 login、logout、push、pull、search

- [ ] **Step 2: 测试未登录的读操作**

Run: `cd iroll && /tmp/logos roll search test`
Expected: 显示搜索结果或提示连接失败（如果 hub 未运行）

- [ ] **Step 3: 测试未登录的写操作**

Run: `cd iroll && /tmp/logos roll push test.iroll test/pkg:v1.0.0`
Expected: 显示 "Not logged in" 错误

- [ ] **Step 4: 更新 README 文档**

在 iroll/readme.md 中添加 irollhub 集成部分：

```markdown
## irollhub 集成

Logos CLI 支持与 irollhub 注册中心集成，用于发现和分享 .iroll 包。

### 登录

```bash
logos roll login [--hub <url>]
```

打开浏览器完成 OAuth 授权，然后输入 API Key。

### 搜索包

```bash
logos roll search <keyword> [--tag <tag>]
```

从 hub 搜索包。

### 拉取包

```bash
logos roll pull <org>/<pkg>[:<ver>]
logos roll pull <org>/<pkg>:<ver> -o <path>
```

下载并加载包（或仅下载）。

### 推送包

```bash
logos roll push <file.iroll|name> <org>/<pkg>:<ver>
```

上传包到 hub。

### 登出

```bash
logos roll logout
```

清除本地凭证。

### 配置

凭证存储在 `~/.iroll/config`（INI 格式）：

```ini
[hub "https://irollhub.example.com"]
    token = iroll_abc123...
```

支持环境变量覆盖：
- `IROLLHUB_URL` - Hub URL
- `IROLLHUB_TOKEN` - API Token
```

- [ ] **Step 5: 验证代码构建**

Run: `cd iroll && go build .`
Expected: 构建成功

- [ ] **Step 6: 最终提交**

Run: `git add iroll/readme.md && git commit -m "docs: add irollhub integration documentation"`
