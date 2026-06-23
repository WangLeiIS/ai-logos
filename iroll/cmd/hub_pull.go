package cmd

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"logos/store"

	"github.com/spf13/cobra"
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
		// 错误检查后保证 resp 非空
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
			if err := store.Extract(tmpFile, name, version); err != nil {
				outputError(fmt.Sprintf("Failed to extract package: %v", err))
			}

			outputJSON(map[string]string{
				"message": fmt.Sprintf("Pulled %s/%s:%s", org, pkg, version),
				"name":    name,
				"version": version,
				"path":    checkedIrollPath(name, version),
			})
		}
	},
}

func init() {
	pullCmd.Flags().StringVarP(&pullOutput, "output", "o", "", "Output file path (default: load to ~/.iroll/)")
	rollCmd.AddCommand(pullCmd)
}
