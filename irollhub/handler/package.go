package handler

import (
	"database/sql"
	"net/http"

	"irollhub/middleware"
	"irollhub/model"
	"irollhub/store"

	"github.com/gin-gonic/gin"
)

type PackageHandler struct {
	db *sql.DB
}

func NewPackageHandler(db *sql.DB) *PackageHandler {
	return &PackageHandler{db: db}
}

func (h *PackageHandler) List(c *gin.Context) {
	orgName := c.Param("org")
	org, err := store.GetOrgByName(h.db, orgName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "code": "INTERNAL"})
		return
	}
	if org == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "organization not found", "code": "NOT_FOUND"})
		return
	}
	limit, offset := pagination(c, 20, 0)
	pkgs, err := store.ListPackages(h.db, org.ID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "code": "INTERNAL"})
		return
	}
	if pkgs == nil {
		pkgs = []model.Package{}
	}
	c.JSON(http.StatusOK, gin.H{"data": pkgs})
}

func (h *PackageHandler) Create(c *gin.Context) {
	authOrg := middleware.GetOrg(c)
	orgName := c.Param("org")
	if authOrg.Name != orgName {
		c.JSON(http.StatusForbidden, gin.H{"error": "not organization owner", "code": "FORBIDDEN"})
		return
	}
	var body struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
		Tags        string `json:"tags"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required", "code": "BAD_REQUEST"})
		return
	}
	if body.Description == "" {
		body.Description = ""
	}
	if body.Tags == "" {
		body.Tags = "[]"
	}
	pkg, err := store.CreatePackage(h.db, authOrg.ID, body.Name, body.Description, body.Tags)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "code": "INTERNAL"})
		return
	}
	c.JSON(http.StatusCreated, pkg)
}

func (h *PackageHandler) Get(c *gin.Context) {
	orgName := c.Param("org")
	pkgName := c.Param("pkg")
	org, err := store.GetOrgByName(h.db, orgName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "code": "INTERNAL"})
		return
	}
	if org == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "organization not found", "code": "NOT_FOUND"})
		return
	}
	pkg, err := store.GetPackage(h.db, org.ID, pkgName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "code": "INTERNAL"})
		return
	}
	if pkg == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "package not found", "code": "NOT_FOUND"})
		return
	}
	versions, _ := store.ListVersions(h.db, pkg.ID, 50, 0)
	if versions == nil {
		versions = []model.Version{}
	}
	c.JSON(http.StatusOK, gin.H{
		"id":          pkg.ID,
		"name":        pkg.Name,
		"description": pkg.Description,
		"tags":        pkg.Tags,
		"downloads":   pkg.Downloads,
		"created_at":  pkg.CreatedAt,
		"updated_at":  pkg.UpdatedAt,
		"versions":    versions,
	})
}

func (h *PackageHandler) Delete(c *gin.Context) {
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
	if err := store.DeletePackage(h.db, authOrg.ID, pkgName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "code": "INTERNAL"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true, "package": pkgName})
}
