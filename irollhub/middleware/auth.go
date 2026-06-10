package middleware

import (
	"database/sql"
	"net/http"
	"strings"

	"irollhub/model"
	"irollhub/store"

	"github.com/gin-gonic/gin"
)

const OrgContextKey = "org"

// AuthRequired returns a Gin middleware that validates Bearer token authentication.
// It extracts the token, hashes it, and looks up the organization. On success,
// the org is stored in the Gin context under OrgContextKey.
func AuthRequired(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header", "code": "UNAUTHORIZED"})
			c.Abort()
			return
		}

		token := strings.TrimPrefix(header, "Bearer ")
		if token == header {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format, use Bearer <token>", "code": "UNAUTHORIZED"})
			c.Abort()
			return
		}

		org, err := store.Authenticate(db, token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid api key", "code": "UNAUTHORIZED"})
			c.Abort()
			return
		}

		c.Set(OrgContextKey, org)
		c.Next()
	}
}

// GetOrg extracts the authenticated organization from the Gin context.
// Returns nil if no organization was set (middleware not applied).
func GetOrg(c *gin.Context) *model.Organization {
	val, exists := c.Get(OrgContextKey)
	if !exists {
		return nil
	}
	org, _ := val.(*model.Organization)
	return org
}
