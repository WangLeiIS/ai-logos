package model

import "time"

type Organization struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Provider   string `json:"provider"`
	ProviderID string `json:"provider_id"`
	Email      string `json:"email,omitempty"`
	AvatarURL  string `json:"avatar_url,omitempty"`
	CreatedAt  string `json:"created_at"`
}

type APIKey struct {
	ID         int64  `json:"id"`
	OrgID      int64  `json:"org_id"`
	KeyHash    string `json:"-"`
	Name       string `json:"name"`
	LastUsedAt string `json:"last_used_at,omitempty"`
	CreatedAt  string `json:"created_at"`
}

type Package struct {
	ID          int64  `json:"id"`
	OrgID       int64  `json:"org_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Tags        string `json:"tags"`
	Downloads   int64  `json:"downloads"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	// Joined fields (populated by queries, not stored in DB)
	OrgName string `json:"org_name,omitempty"`
}

type Version struct {
	ID        int64  `json:"id"`
	PackageID int64  `json:"package_id"`
	Version   string `json:"version"`
	ObjectKey string `json:"object_key"`
	FileSize  int64  `json:"file_size"`
	Checksum  string `json:"checksum"`
	CreatedAt string `json:"created_at"`
}

// MinIOConfig holds MinIO connection settings. Used by config and store packages.
type MinIOConfig struct {
	Endpoint  string `yaml:"endpoint"`
	AccessKey string `yaml:"access_key"`
	SecretKey string `yaml:"secret_key"`
	Bucket    string `yaml:"bucket"`
	UseSSL    bool   `yaml:"use_ssl"`
}

// OAuthConfig holds OAuth provider settings. Used by config and handler packages.
type OAuthConfig struct {
	GithubClientID     string `yaml:"github_client_id"`
	GithubClientSecret string `yaml:"github_client_secret"`
	GoogleClientID     string `yaml:"google_client_id"`
	GoogleClientSecret string `yaml:"google_client_secret"`
	RedirectBase       string `yaml:"redirect_base"`
}

func NowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}
