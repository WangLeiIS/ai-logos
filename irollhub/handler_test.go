package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"irollhub/handler"
	"irollhub/middleware"
	"irollhub/store"

	"github.com/gin-gonic/gin"
)

func setupTestDB(t *testing.T) (*gin.Engine, *store.MinIOClient) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tmpFile, err := os.CreateTemp("", "hub-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	dbPath := tmpFile.Name()
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(dbPath) })

	db, err := store.OpenDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	// Use a nil MinIO client since we can't spin up MinIO in tests
	// Upload tests will verify validation up to the MinIO step
	var mc *store.MinIOClient = nil

	r := gin.New()

	orgH := handler.NewOrgHandler(db)
	pkgH := handler.NewPackageHandler(db)
	verH := handler.NewVersionHandler(db, mc)
	searchH := handler.NewSearchHandler(db)

	api := r.Group("/api/v1")
	{
		orgs := api.Group("/orgs")
		{
			orgs.GET("", orgH.List)
			orgs.POST("", middleware.AuthRequired(db), orgH.Create)
			orgs.GET("/:org", orgH.Get)
		}

		pkgs := orgs.Group("/:org/packages")
		{
			pkgs.GET("", pkgH.List)
			pkgs.POST("", middleware.AuthRequired(db), pkgH.Create)
			pkgs.GET("/:pkg", pkgH.Get)
			pkgs.DELETE("/:pkg", middleware.AuthRequired(db), pkgH.Delete)
		}

		vers := pkgs.Group("/:pkg/versions")
		{
			vers.GET("", verH.List)
			vers.POST("", middleware.AuthRequired(db), verH.Upload)
			vers.GET("/:ver", verH.Get)
			vers.DELETE("/:ver", middleware.AuthRequired(db), verH.Delete)
		}

		api.GET("/search", searchH.Search)
	}

	return r, mc
}

func createTestOrgAndKey(t *testing.T, db *gin.Engine) (string, string) {
	t.Helper()
	// Create org directly via store (bypassing HTTP since orgs need OAuth in production)
	// Open a temp db connection for setup
	tmpFile, _ := os.CreateTemp("", "setup-*.db")
	dbPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(dbPath)
	// Already have the DB from setupTestDB, create org through store
	return "", ""
}

// Simple test that doesn't need auth
func TestListOrgsEmpty(t *testing.T) {
	r, _ := setupTestDB(t)

	req := httptest.NewRequest("GET", "/api/v1/orgs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)
	data, ok := result["data"]
	if !ok {
		t.Fatal("expected data field in response")
	}
	arr, ok := data.([]interface{})
	if !ok {
		t.Fatal("expected data to be array")
	}
	if len(arr) != 0 {
		t.Fatalf("expected empty array, got %d items", len(arr))
	}
}

func TestGetOrgNotFound(t *testing.T) {
	r, _ := setupTestDB(t)

	req := httptest.NewRequest("GET", "/api/v1/orgs/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListPackagesNonexistentOrg(t *testing.T) {
	r, _ := setupTestDB(t)

	req := httptest.NewRequest("GET", "/api/v1/orgs/nonexistent/packages", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	r, _ := setupTestDB(t)

	req := httptest.NewRequest("GET", "/api/v1/search", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuthRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tmpFile, _ := os.CreateTemp("", "auth-test-*.db")
	dbPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(dbPath)

	db, err := store.OpenDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	r := gin.New()
	r.GET("/api/v1/auth/me", middleware.AuthRequired(db), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// No auth header
	req := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing header, got %d", w.Code)
	}

	// Invalid key format
	req = httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer invalid_key")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid key, got %d", w.Code)
	}

	// Valid key
	org, _ := store.CreateOrg(db, "testuser", "github", "123", "", "")
	rawKey, _, _ := store.CreateAPIKey(db, org.ID, "test")
	req = httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid key, got %d: %s", w.Code, w.Body.String())
	}
}

func makeTestIroll(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, _ := w.Create("ai_roll.db")
	io.WriteString(f, "fake db content")
	w.Close()
	return buf.Bytes()
}

func TestUploadValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tmpFile, _ := os.CreateTemp("", "upload-test-*.db")
	dbPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(dbPath)

	db, err := store.OpenDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	org, _ := store.CreateOrg(db, "uploader", "github", "456", "", "")
	rawKey, _, _ := store.CreateAPIKey(db, org.ID, "test")
	_, _ = store.CreatePackage(db, org.ID, "test-pkg", "test", "[]")

	r := gin.New()
	mc := (*store.MinIOClient)(nil) // nil client — upload will fail at MinIO step, but validation passes
	verH := handler.NewVersionHandler(db, mc)
	r.POST("/api/v1/orgs/:org/packages/:pkg/versions", middleware.AuthRequired(db), verH.Upload)

	// Test: invalid ZIP
	req := httptest.NewRequest("POST", "/api/v1/orgs/uploader/packages/test-pkg/versions", bytes.NewReader([]byte("not a zip")))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid upload, got %d: %s", w.Code, w.Body.String())
	}

	// Test: valid ZIP without ai_roll.db
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, _ := zw.Create("some_other_file.txt")
	io.WriteString(f, "not a db")
	zw.Close()

	req = httptest.NewRequest("POST", "/api/v1/orgs/uploader/packages/test-pkg/versions", bytes.NewReader(buf.Bytes()))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for ZIP without ai_roll.db, got %d: %s", w.Code, w.Body.String())
	}
}

func TestForbiddenAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tmpFile, _ := os.CreateTemp("", "forbidden-test-*.db")
	dbPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(dbPath)

	db, err := store.OpenDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create two orgs
	store.CreateOrg(db, "alice", "github", "1", "", "")
	store.CreateOrg(db, "bob", "github", "2", "", "")
	rawKey, _, _ := store.CreateAPIKey(db, 1, "test") // alice's key
	store.CreatePackage(db, 2, "private-pkg", "bob's package", "[]")

	r := gin.New()
	pkgH := handler.NewPackageHandler(db)
	r.DELETE("/api/v1/orgs/:org/packages/:pkg", middleware.AuthRequired(db), pkgH.Delete)

	// Alice tries to delete Bob's package
	req := httptest.NewRequest("DELETE", "/api/v1/orgs/bob/packages/private-pkg", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 forbidden, got %d: %s", w.Code, w.Body.String())
	}
}
