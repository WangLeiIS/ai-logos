package cmd

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"logos/store"
)

// createE2EIrollZip creates a minimal valid .iroll ZIP at path containing ai_roll.db.
func createE2EIrollZip(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	out, err := w.Create("ai_roll.db")
	if err != nil {
		t.Fatalf("create ai_roll.db entry: %v", err)
	}
	if _, err := out.Write([]byte("fake-db-content")); err != nil {
		t.Fatalf("write ai_roll.db: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
}

// createE2EIrollZipBytes creates a minimal valid .iroll ZIP and returns its bytes.
func createE2EIrollZipBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	out, err := w.Create("ai_roll.db")
	if err != nil {
		t.Fatalf("create ai_roll.db entry: %v", err)
	}
	if _, err := out.Write([]byte("fake-db-content")); err != nil {
		t.Fatalf("write ai_roll.db: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	return buf.Bytes()
}

// setTempHome redirects HOME and USERPROFILE to tmpDir so config and store
// operations use an isolated directory.
func setTempHome(t *testing.T, tmpDir string) {
	t.Helper()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)
}

// TestHubLoginWritesConfig verifies writeConfig/readConfig round-trip and logout.
func TestHubLoginWritesConfig(t *testing.T) {
	tmpDir := t.TempDir()
	setTempHome(t, tmpDir)

	// Clear any env var overrides so the test reads from the file.
	t.Setenv("IROLLHUB_URL", "")
	t.Setenv("IROLLHUB_TOKEN", "")

	hubURL := "https://hub.example.com"
	token := "secret-token-abc"

	// Write config (login).
	if err := writeConfig(hubURL, token); err != nil {
		t.Fatalf("writeConfig login: %v", err)
	}

	cfg, err := readConfig()
	if err != nil {
		t.Fatalf("readConfig after login: %v", err)
	}
	if cfg.HubURL != hubURL {
		t.Errorf("HubURL mismatch: got %q, want %q", cfg.HubURL, hubURL)
	}
	if cfg.Token != token {
		t.Errorf("Token mismatch: got %q, want %q", cfg.Token, token)
	}

	// Write config with empty token (logout).
	if err := writeConfig(hubURL, ""); err != nil {
		t.Fatalf("writeConfig logout: %v", err)
	}

	cfg, err = readConfig()
	if err != nil {
		t.Fatalf("readConfig after logout: %v", err)
	}
	if cfg.Token != "" {
		t.Errorf("Token should be empty after logout, got %q", cfg.Token)
	}
}

// TestHubPushValidatesAndUploads validates a .iroll ZIP, computes checksum, and
// uploads via a mock HTTP server that verifies the POST + Authorization header.
func TestHubPushValidatesAndUploads(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test.iroll")
	createE2EIrollZip(t, zipPath)

	// validateIrollZip should pass.
	if err := validateIrollZip(zipPath); err != nil {
		t.Fatalf("validateIrollZip: %v", err)
	}

	// computeChecksum should return non-empty values.
	checksum, size, err := computeChecksum(zipPath)
	if err != nil {
		t.Fatalf("computeChecksum: %v", err)
	}
	if checksum == "" {
		t.Error("checksum is empty")
	}
	if size == 0 {
		t.Error("size is 0")
	}

	// Mock server that checks POST method and Authorization header.
	var gotMethod, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"message":  "uploaded",
			"checksum": checksum,
		})
	}))
	defer server.Close()

	client := NewHubClient(server.URL, "my-push-token")
	body := bytes.NewReader([]byte("dummy-body"))
	resp, err := client.Post("/api/v1/orgs/testorg/packages/testpkg/versions", body, "application/octet-stream")
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	defer resp.Body.Close()

	if gotMethod != "POST" {
		t.Errorf("method: got %q, want POST", gotMethod)
	}
	if gotAuth != "Bearer my-push-token" {
		t.Errorf("auth header: got %q, want %q", gotAuth, "Bearer my-push-token")
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusCreated)
	}
}

// TestHubPullDownloadsAndExtracts downloads ZIP bytes from a mock server,
// writes to a temp file, then extracts with store.Extract.
func TestHubPullDownloadsAndExtracts(t *testing.T) {
	tmpDir := t.TempDir()
	setTempHome(t, tmpDir)

	// Clear env var overrides.
	t.Setenv("IROLLHUB_URL", "")
	t.Setenv("IROLLHUB_TOKEN", "")

	zipBytes := createE2EIrollZipBytes(t)

	// Mock server returns the ZIP bytes.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Write(zipBytes)
	}))
	defer server.Close()

	client := NewHubClient(server.URL, "")
	data, resp, err := client.Download("/api/v1/orgs/testorg/packages/testpkg/versions/latest/download")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Write downloaded bytes to a temp .iroll file.
	tmpZip := filepath.Join(tmpDir, "pulled.iroll")
	if err := os.WriteFile(tmpZip, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Extract using store.Extract.
	if err := store.Extract(tmpZip, "pulled-agent", "latest"); err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// Verify ai_roll.db exists in the extracted directory.
	extractedDB := filepath.Join(tmpDir, ".iroll", "pulled-agent", "latest", "ai_roll.db")
	if _, err := os.Stat(extractedDB); err != nil {
		t.Errorf("ai_roll.db not found at %s: %v", extractedDB, err)
	}
}

// TestHubSearchQueriesAPI hits a mock search endpoint and decodes JSON results.
func TestHubSearchQueriesAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify it's a search request.
		if r.URL.Path != "/api/v1/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		q := r.URL.Query().Get("q")
		if q != "test-query" {
			t.Errorf("unexpected query param q: %s", q)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{
				{
					"org":       "testorg",
					"package":   "testpkg",
					"version":   "v1.0.0",
					"downloads": 42,
				},
				{
					"org":       "otherorg",
					"package":   "otherpkg",
					"version":   "v2.0.0",
					"downloads": 7,
				},
			},
		})
	}))
	defer server.Close()

	client := NewHubClient(server.URL, "search-token")
	resp, err := client.Get("/api/v1/search?q=test-query")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	var result struct {
		Results []struct {
			Org       string `json:"org"`
			Package   string `json:"package"`
			Version   string `json:"version"`
			Downloads int    `json:"downloads"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(result.Results) != 2 {
		t.Fatalf("results count: got %d, want 2", len(result.Results))
	}

	if result.Results[0].Org != "testorg" {
		t.Errorf("result[0].Org: got %q, want %q", result.Results[0].Org, "testorg")
	}
	if result.Results[0].Downloads != 42 {
		t.Errorf("result[0].Downloads: got %d, want 42", result.Results[0].Downloads)
	}
	if result.Results[1].Version != "v2.0.0" {
		t.Errorf("result[1].Version: got %q, want %q", result.Results[1].Version, "v2.0.0")
	}
}

// TestHubConfigFromEnvironment verifies that env vars take precedence over file config.
func TestHubConfigFromEnvironment(t *testing.T) {
	tmpDir := t.TempDir()
	setTempHome(t, tmpDir)

	// Write a file config with different values.
	if err := writeConfig("https://file-url.example.com", "file-token-xyz"); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}

	// Set env vars — these should take priority over the file.
	envURL := "https://env-url.example.com"
	envToken := "env-token-123"
	t.Setenv("IROLLHUB_URL", envURL)
	t.Setenv("IROLLHUB_TOKEN", envToken)

	cfg, err := readConfig()
	if err != nil {
		t.Fatalf("readConfig: %v", err)
	}

	if cfg.HubURL != envURL {
		t.Errorf("HubURL: got %q, want %q (from env)", cfg.HubURL, envURL)
	}
	if cfg.Token != envToken {
		t.Errorf("Token: got %q, want %q (from env)", cfg.Token, envToken)
	}
}
