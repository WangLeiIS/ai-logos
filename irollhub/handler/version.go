package handler

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"

	"irollhub/middleware"
	"irollhub/model"
	"irollhub/store"

	"github.com/gin-gonic/gin"
)

var validVersion = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,99}$`)

const maxUploadSize = 500 << 20 // 500MB

type VersionHandler struct {
	db *sql.DB
	mc *store.MinIOClient
}

func NewVersionHandler(db *sql.DB, mc *store.MinIOClient) *VersionHandler {
	return &VersionHandler{db: db, mc: mc}
}

func (h *VersionHandler) List(c *gin.Context) {
	orgName := c.Param("org")
	pkgName := c.Param("pkg")
	org, err := store.GetOrgByName(h.db, orgName)
	if err != nil || org == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "organization not found", "code": "NOT_FOUND"})
		return
	}
	pkg, err := store.GetPackage(h.db, org.ID, pkgName)
	if err != nil || pkg == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "package not found", "code": "NOT_FOUND"})
		return
	}
	limit, offset := pagination(c, 20, 0)
	versions, err := store.ListVersions(h.db, pkg.ID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error", "code": "INTERNAL"})
		return
	}
	if versions == nil {
		versions = []model.Version{}
	}
	c.JSON(http.StatusOK, gin.H{"data": versions})
}

func (h *VersionHandler) Upload(c *gin.Context) {
	authOrg := middleware.GetOrg(c)
	orgName := c.Param("org")
	if authOrg.Name != orgName {
		c.JSON(http.StatusForbidden, gin.H{"error": "not organization owner", "code": "FORBIDDEN"})
		return
	}
	pkgName := c.Param("pkg")
	pkg, err := store.GetPackage(h.db, authOrg.ID, pkgName)
	if err != nil || pkg == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "package not found", "code": "NOT_FOUND"})
		return
	}

	// Limit request body size
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadSize)

	// Parse multipart form
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file upload error: " + err.Error(), "code": "BAD_REQUEST"})
		return
	}
	defer file.Close()

	if header.Size > maxUploadSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file exceeds 500MB limit", "code": "BAD_REQUEST"})
		return
	}

	// Read into memory for validation + upload
	data, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "read file error", "code": "BAD_REQUEST"})
		return
	}

	// Validate ZIP
	zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not a valid ZIP archive", "code": "BAD_REQUEST"})
		return
	}

	// Validate contains ai_roll.db
	hasDB := false
	for _, f := range zipReader.File {
		if f.Name == "ai_roll.db" {
			hasDB = true
			break
		}
	}
	if !hasDB {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ZIP must contain ai_roll.db", "code": "BAD_REQUEST"})
		return
	}

	version := c.PostForm("version")
	if version == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "version is required", "code": "BAD_REQUEST"})
		return
	}
	if !validVersion.MatchString(version) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid version format", "code": "BAD_REQUEST"})
		return
	}

	// Check version conflict
	existing, err := store.GetVersion(h.db, pkg.ID, version)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error", "code": "INTERNAL"})
		return
	}
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("version %s already exists", version), "code": "CONFLICT"})
		return
	}

	// Checksum
	hash := sha256.Sum256(data)
	checksum := hex.EncodeToString(hash[:])

	// Upload to MinIO
	objectKey := fmt.Sprintf("%s/%s/%s.iroll", orgName, pkgName, version)
	if err := h.mc.Upload(c.Request.Context(), objectKey, bytes.NewReader(data), int64(len(data))); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "upload to storage failed", "code": "INTERNAL"})
		return
	}

	// Write to SQLite
	ver, err := store.CreateVersion(h.db, pkg.ID, version, objectKey, int64(len(data)), checksum)
	if err != nil {
		// Cleanup MinIO on DB failure
		h.mc.Delete(c.Request.Context(), objectKey)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error", "code": "INTERNAL"})
		return
	}

	c.JSON(http.StatusCreated, ver)
}

func (h *VersionHandler) Get(c *gin.Context) {
	orgName := c.Param("org")
	pkgName := c.Param("pkg")
	verName := c.Param("ver")
	org, err := store.GetOrgByName(h.db, orgName)
	if err != nil || org == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "organization not found", "code": "NOT_FOUND"})
		return
	}
	pkg, err := store.GetPackage(h.db, org.ID, pkgName)
	if err != nil || pkg == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "package not found", "code": "NOT_FOUND"})
		return
	}
	ver, err := store.GetVersion(h.db, pkg.ID, verName)
	if err != nil || ver == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found", "code": "NOT_FOUND"})
		return
	}
	c.JSON(http.StatusOK, ver)
}

func (h *VersionHandler) Download(c *gin.Context) {
	orgName := c.Param("org")
	pkgName := c.Param("pkg")
	verName := c.Param("ver")
	org, err := store.GetOrgByName(h.db, orgName)
	if err != nil || org == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "organization not found", "code": "NOT_FOUND"})
		return
	}
	pkg, err := store.GetPackage(h.db, org.ID, pkgName)
	if err != nil || pkg == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "package not found", "code": "NOT_FOUND"})
		return
	}

	// Resolve "latest" version
	var ver *model.Version
	if verName == "latest" {
		ver, err = store.GetLatestVersion(h.db, pkg.ID)
		if err != nil || ver == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "no versions found", "code": "NOT_FOUND"})
			return
		}
	} else {
		ver, err = store.GetVersion(h.db, pkg.ID, verName)
		if err != nil || ver == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "version not found", "code": "NOT_FOUND"})
			return
		}
	}

	store.IncrementDownloads(h.db, pkg.ID)

	url, err := h.mc.PresignedGetURL(c.Request.Context(), ver.ObjectKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "generate download url failed", "code": "INTERNAL"})
		return
	}
	c.Redirect(http.StatusFound, url)
}

func (h *VersionHandler) Delete(c *gin.Context) {
	authOrg := middleware.GetOrg(c)
	orgName := c.Param("org")
	if authOrg.Name != orgName {
		c.JSON(http.StatusForbidden, gin.H{"error": "not organization owner", "code": "FORBIDDEN"})
		return
	}
	pkgName := c.Param("pkg")
	verName := c.Param("ver")
	pkg, err := store.GetPackage(h.db, authOrg.ID, pkgName)
	if err != nil || pkg == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "package not found", "code": "NOT_FOUND"})
		return
	}
	ver, err := store.GetVersion(h.db, pkg.ID, verName)
	if err != nil || ver == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found", "code": "NOT_FOUND"})
		return
	}
	if err := store.DeleteVersion(h.db, pkg.ID, verName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error", "code": "INTERNAL"})
		return
	}
	if err := h.mc.Delete(c.Request.Context(), ver.ObjectKey); err != nil {
		log.Printf("delete minio object %s: %v", ver.ObjectKey, err)
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true, "version": verName})
}
