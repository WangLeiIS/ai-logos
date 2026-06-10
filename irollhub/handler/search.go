package handler

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"irollhub/model"
	"irollhub/store"

	"github.com/gin-gonic/gin"
)

func sanitizeFTS5Query(q string) string {
	q = strings.Map(func(r rune) rune {
		switch r {
		case '"', '*', '(', ')', '-':
			return -1
		}
		return r
	}, q)
	q = strings.TrimSpace(q)
	if q == "" {
		return q
	}
	return `"` + q + `"`
}

type SearchHandler struct {
	db *sql.DB
}

func NewSearchHandler(db *sql.DB) *SearchHandler {
	return &SearchHandler{db: db}
}

func (h *SearchHandler) Search(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "q parameter is required", "code": "BAD_REQUEST"})
		return
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit < 1 || limit > 100 {
		limit = 20
	}

	safeQ := sanitizeFTS5Query(q)
	if safeQ == "" {
		c.JSON(http.StatusOK, gin.H{"data": []model.Package{}})
		return
	}
	pkgs, err := store.SearchPackages(h.db, safeQ, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error", "code": "INTERNAL"})
		return
	}
	if pkgs == nil {
		pkgs = []model.Package{}
	}
	c.JSON(http.StatusOK, gin.H{"data": pkgs})
}
