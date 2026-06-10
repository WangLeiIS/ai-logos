package handler

import (
	"database/sql"
	"net/http"
	"strconv"

	"irollhub/middleware"
	"irollhub/model"
	"irollhub/store"

	"github.com/gin-gonic/gin"
)

type OrgHandler struct {
	db *sql.DB
}

func NewOrgHandler(db *sql.DB) *OrgHandler {
	return &OrgHandler{db: db}
}

func (h *OrgHandler) List(c *gin.Context) {
	limit, offset := pagination(c, 20, 0)
	orgs, err := store.ListOrgs(h.db, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error", "code": "INTERNAL"})
		return
	}
	if orgs == nil {
		orgs = []model.Organization{}
	}
	c.JSON(http.StatusOK, gin.H{"data": orgs})
}

func (h *OrgHandler) Create(c *gin.Context) {
	// Orgs are auto-created during OAuth. This endpoint exists for completeness.
	org := middleware.GetOrg(c)
	c.JSON(http.StatusOK, org)
}

func (h *OrgHandler) Get(c *gin.Context) {
	name := c.Param("org")
	org, err := store.GetOrgByName(h.db, name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error", "code": "INTERNAL"})
		return
	}
	if org == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "organization not found", "code": "NOT_FOUND"})
		return
	}
	pkgs, _ := store.ListPackages(h.db, org.ID, 100, 0)
	if pkgs == nil {
		pkgs = []model.Package{}
	}
	c.JSON(http.StatusOK, gin.H{
		"id":         org.ID,
		"name":       org.Name,
		"avatar_url": org.AvatarURL,
		"created_at": org.CreatedAt,
		"packages":   pkgs,
	})
}

// pagination extracts limit and offset from query params with defaults.
func pagination(c *gin.Context, defaultLimit, defaultOffset int) (int, int) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", strconv.Itoa(defaultLimit)))
	if err != nil || limit < 1 || limit > 100 {
		limit = defaultLimit
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", strconv.Itoa(defaultOffset)))
	if err != nil || offset < 0 {
		offset = defaultOffset
	}
	return limit, offset
}
