package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"irollhub/middleware"
	"irollhub/store"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"
)

type AuthHandler struct {
	db          *sql.DB
	githubOAuth *oauth2.Config
	googleOAuth *oauth2.Config
}

func NewAuthHandler(db *sql.DB, githubClientID, githubClientSecret, googleClientID, googleClientSecret, redirectBase string) *AuthHandler {
	return &AuthHandler{
		db: db,
		githubOAuth: &oauth2.Config{
			ClientID:     githubClientID,
			ClientSecret: githubClientSecret,
			Endpoint:     github.Endpoint,
			Scopes:       []string{"read:user", "user:email"},
			RedirectURL:  redirectBase + "/api/v1/auth/github/callback",
		},
		googleOAuth: &oauth2.Config{
			ClientID:     googleClientID,
			ClientSecret: googleClientSecret,
			Endpoint:     google.Endpoint,
			Scopes:       []string{"openid", "email", "profile"},
			RedirectURL:  redirectBase + "/api/v1/auth/google/callback",
		},
	}
}

// GitHub OAuth

func (h *AuthHandler) GithubStart(c *gin.Context) {
	url := h.githubOAuth.AuthCodeURL("state", oauth2.AccessTypeOffline)
	c.Redirect(http.StatusFound, url)
}

func (h *AuthHandler) GithubCallback(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing code parameter", "code": "BAD_REQUEST"})
		return
	}
	token, err := h.githubOAuth.Exchange(context.Background(), code)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "github oauth exchange failed", "code": "BAD_REQUEST"})
		return
	}
	userInfo, err := fetchGithubUser(token.AccessToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "fetch github user failed", "code": "INTERNAL"})
		return
	}
	h.handleOAuthCallback(c, "github", fmt.Sprintf("%d", userInfo.ID), userInfo.Login, userInfo.Email, userInfo.AvatarURL)
}

// Google OAuth

func (h *AuthHandler) GoogleStart(c *gin.Context) {
	url := h.googleOAuth.AuthCodeURL("state", oauth2.AccessTypeOffline)
	c.Redirect(http.StatusFound, url)
}

func (h *AuthHandler) GoogleCallback(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing code parameter", "code": "BAD_REQUEST"})
		return
	}
	token, err := h.googleOAuth.Exchange(context.Background(), code)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "google oauth exchange failed", "code": "BAD_REQUEST"})
		return
	}
	userInfo, err := fetchGoogleUser(token.AccessToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "fetch google user failed", "code": "INTERNAL"})
		return
	}
	name := extractLocalPart(userInfo.Email)
	h.handleOAuthCallback(c, "google", userInfo.Sub, name, userInfo.Email, userInfo.Picture)
}

// Shared callback logic

func (h *AuthHandler) handleOAuthCallback(c *gin.Context, provider, providerID, name, email, avatarURL string) {
	org, err := store.FindOrgByProvider(h.db, provider, providerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error", "code": "INTERNAL"})
		return
	}
	if org == nil {
		uniqueName, err := store.GenerateUniqueOrgName(h.db, name)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db error", "code": "INTERNAL"})
			return
		}
		org, err = store.CreateOrg(h.db, uniqueName, provider, providerID, email, avatarURL)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "create org failed", "code": "INTERNAL"})
			return
		}
	}
	rawKey, _, err := store.CreateAPIKey(h.db, org.ID, "oauth-issued")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create key failed", "code": "INTERNAL"})
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(fmt.Sprintf(callbackHTML, rawKey)))
}

const callbackHTML = `<!DOCTYPE html>
<html><head><title>irollhub - API Key</title>
<meta charset="utf-8">
<style>body{font-family:monospace;max-width:520px;margin:80px auto;padding:0 20px;background:#0d1117;color:#c9d1d9}
h2{color:#58a6ff}.key{background:#161b22;color:#3fb950;padding:16px;border-radius:6px;border:1px solid #30363d;word-break:break-all;margin:16px 0}
button{padding:8px 16px;background:#238636;color:#fff;border:none;border-radius:6px;cursor:pointer;font-size:14px}
button:hover{background:#2ea043}p.warn{color:#8b949e;font-size:12px}</style></head>
<body><h2>irollhub API Key</h2>
<p>Copy this key and configure it in your CLI:</p>
<div class="key" id="key">%s</div>
<button onclick="navigator.clipboard.writeText(document.getElementById('key').textContent);this.textContent='Copied!'">Copy</button>
<p class="warn">This key will only be shown once. Save it now.</p>
</body></html>`

// API Key management

func (h *AuthHandler) Me(c *gin.Context) {
	org := middleware.GetOrg(c)
	c.JSON(http.StatusOK, org)
}

func (h *AuthHandler) CreateKey(c *gin.Context) {
	org := middleware.GetOrg(c)
	var body struct {
		Name string `json:"name"`
	}
	c.ShouldBindJSON(&body)
	if body.Name == "" {
		body.Name = "default"
	}

	rawKey, apiKey, err := store.CreateAPIKey(h.db, org.ID, body.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "code": "INTERNAL"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"id":         apiKey.ID,
		"name":       apiKey.Name,
		"key":        rawKey,
		"created_at": apiKey.CreatedAt,
		"warning":    "This key will only be shown once.",
	})
}

func (h *AuthHandler) RevokeKey(c *gin.Context) {
	org := middleware.GetOrg(c)
	keyID, err := strconv.ParseInt(c.Param("key_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key_id", "code": "BAD_REQUEST"})
		return
	}
	if err := store.DeleteAPIKey(h.db, keyID, org.ID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error(), "code": "NOT_FOUND"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// OAuth user fetching helpers

type githubUser struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

type githubEmail struct {
	Email   string `json:"email"`
	Primary bool   `json:"primary"`
}

type googleUser struct {
	Sub     string `json:"sub"`
	Email   string `json:"email"`
	Picture string `json:"picture"`
}

func fetchGithubUser(token string) (*githubUser, error) {
	req, _ := http.NewRequest("GET", "https://api.github.com/user", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var u githubUser
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, err
	}
	if u.Email == "" {
		u.Email = fetchGithubEmail(token)
	}
	return &u, nil
}

func fetchGithubEmail(token string) string {
	req, _ := http.NewRequest("GET", "https://api.github.com/user/emails", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var emails []githubEmail
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return ""
	}
	for _, e := range emails {
		if e.Primary {
			return e.Email
		}
	}
	if len(emails) > 0 {
		return emails[0].Email
	}
	return ""
}

func fetchGoogleUser(token string) (*googleUser, error) {
	req, _ := http.NewRequest("GET", "https://www.googleapis.com/oauth2/v3/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var u googleUser
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, err
	}
	return &u, nil
}

func extractLocalPart(email string) string {
	local, _, _ := strings.Cut(email, "@")
	if local == "" {
		return "user"
	}
	return local
}
