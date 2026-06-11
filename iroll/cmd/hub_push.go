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

	"github.com/spf13/cobra"
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

		writer.WriteField("version", version)
		writer.Close()

		// 发送请求
		uploadPath := fmt.Sprintf("/api/v1/orgs/%s/packages/%s/versions", org, pkg)

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

		outputJSON(map[string]interface{}{
			"message":  fmt.Sprintf("Pushed %s/%s:%s", org, pkg, version),
			"size":     fileSize,
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

		// 在 ZIP 中创建文件
		dest, err := zipWriter.Create(relPath)
		if err != nil {
			srcFile.Close()
			return err
		}

		// 复制内容
		_, err = io.Copy(dest, srcFile)
		srcFile.Close()
		return err
	})
}

func init() {
	rollCmd.AddCommand(pushCmd)
}
