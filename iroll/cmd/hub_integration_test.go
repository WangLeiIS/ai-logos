package cmd

import (
	"archive/zip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigReadWrite(t *testing.T) {
	// 清理可能存在的配置文件
	os.Remove(configPath())

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

	// 清理
	os.Remove(configPath())
}

func TestIsNetworkError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantNet bool
	}{
		{"nil error", nil, false},
		{"non-network error", os.ErrNotExist, false},
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

func TestValidateIrollZip(t *testing.T) {
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
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

	if includeDB {
		w, err := zipWriter.Create("ai_roll.db")
		if err != nil {
			return err
		}
		w.Write([]byte(""))
	}

	return nil
}

func TestHubClient(t *testing.T) {
	// 创建测试服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/me" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"name": "testuser"})
			return
		}
		if r.URL.Path == "/api/v1/search" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{
					{"org": "test", "package": "pkg", "version": "v1.0.0", "downloads": 42},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewHubClient(server.URL, "test-token")

	// 测试 GET
	resp, err := client.Get("/api/v1/auth/me")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestParseAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"error": "bad request", "code": "INVALID"}`))
	}))
	defer server.Close()

	client := NewHubClient(server.URL, "")
	resp, err := client.Get("/test")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	apiErr := parseAPIError(resp)
	if apiErr == nil {
		t.Fatal("Expected error, got nil")
	}
}

func TestNewHubClient(t *testing.T) {
	// 测试默认 URL
	client := NewHubClient("", "")
	if client.BaseURL != DefaultHubURL {
		t.Errorf("Expected %s, got %s", DefaultHubURL, client.BaseURL)
	}

	// 测试自定义 URL
	client = NewHubClient("https://custom.example.com", "token")
	if client.BaseURL != "https://custom.example.com" {
		t.Errorf("Expected custom URL, got %s", client.BaseURL)
	}

	// 测试 URL 尾部斜杠
	client = NewHubClient("https://custom.example.com/", "")
	if client.BaseURL != "https://custom.example.com" {
		t.Errorf("Expected trimmed URL, got %s", client.BaseURL)
	}
}
