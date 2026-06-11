package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gopkg.in/ini.v1"
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

	// Check for timeout errors
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}

	// Check for temporary errors
	if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
		return true
	}

	// Check for specific error types
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}

	// Fallback to string matching
	errStr := strings.ToLower(err.Error())
	networkKeywords := []string{"connection refused", "no such host", "connection reset"}
	for _, keyword := range networkKeywords {
		if strings.Contains(errStr, keyword) {
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
			hubName := strings.TrimPrefix(section.Name(), "hub ")
			hubName = strings.Trim(hubName, "\"")
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
