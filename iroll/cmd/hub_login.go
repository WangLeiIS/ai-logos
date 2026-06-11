package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
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
		// resp is guaranteed non-nil after successful retry
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
