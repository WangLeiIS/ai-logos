package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"strings"

	"irollhub/model"

	_ "github.com/mattn/go-sqlite3"
)

func OpenDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", path+"?_foreign_keys=on")
	if err != nil {
		return nil, err
	}
	if err := ensureSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func ensureSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS organizations (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			name        TEXT    NOT NULL UNIQUE,
			provider    TEXT    NOT NULL,
			provider_id TEXT    NOT NULL,
			email       TEXT,
			avatar_url  TEXT,
			created_at  TEXT    NOT NULL
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_orgs_provider ON organizations(provider, provider_id);

		CREATE TABLE IF NOT EXISTS api_keys (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			org_id       INTEGER NOT NULL REFERENCES organizations(id),
			key_hash     TEXT    NOT NULL UNIQUE,
			name         TEXT    NOT NULL DEFAULT 'default',
			last_used_at TEXT,
			created_at   TEXT    NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_api_keys_org ON api_keys(org_id);

		CREATE TABLE IF NOT EXISTS packages (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			org_id      INTEGER NOT NULL REFERENCES organizations(id),
			name        TEXT    NOT NULL,
			description TEXT    NOT NULL DEFAULT '',
			tags        TEXT    NOT NULL DEFAULT '[]',
			downloads   INTEGER NOT NULL DEFAULT 0,
			created_at  TEXT    NOT NULL,
			updated_at  TEXT    NOT NULL
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_packages_org_name ON packages(org_id, name);

		CREATE TABLE IF NOT EXISTS versions (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			package_id  INTEGER NOT NULL REFERENCES packages(id),
			version     TEXT    NOT NULL,
			object_key  TEXT    NOT NULL,
			file_size   INTEGER NOT NULL,
			checksum    TEXT    NOT NULL,
			created_at  TEXT    NOT NULL
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_versions_pkg_ver ON versions(package_id, version);

		CREATE VIRTUAL TABLE IF NOT EXISTS packages_fts USING fts5(
			name,
			description,
			tags,
			content=packages,
			content_rowid=id
		);

		CREATE TRIGGER IF NOT EXISTS packages_ai AFTER INSERT ON packages BEGIN
			INSERT INTO packages_fts(rowid, name, description, tags) VALUES (new.id, new.name, new.description, new.tags);
		END;

		CREATE TRIGGER IF NOT EXISTS packages_ad AFTER DELETE ON packages BEGIN
			INSERT INTO packages_fts(packages_fts, rowid, name, description, tags) VALUES('delete', old.id, old.name, old.description, old.tags);
		END;

		CREATE TRIGGER IF NOT EXISTS packages_au AFTER UPDATE ON packages BEGIN
			INSERT INTO packages_fts(packages_fts, rowid, name, description, tags) VALUES('delete', old.id, old.name, old.description, old.tags);
			INSERT INTO packages_fts(rowid, name, description, tags) VALUES (new.id, new.name, new.description, new.tags);
		END;
	`)
	return err
}

func CreateOrg(db *sql.DB, name, provider, providerID, email, avatarURL string) (*model.Organization, error) {
	now := model.NowISO()
	res, err := db.Exec(
		"INSERT INTO organizations (name, provider, provider_id, email, avatar_url, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		name, provider, providerID, email, avatarURL, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create org: %w", err)
	}
	id, _ := res.LastInsertId()
	return &model.Organization{ID: id, Name: name, Provider: provider, ProviderID: providerID, Email: email, AvatarURL: avatarURL, CreatedAt: now}, nil
}

func FindOrgByProvider(db *sql.DB, provider, providerID string) (*model.Organization, error) {
	var o model.Organization
	var email, avatar sql.NullString
	err := db.QueryRow(
		"SELECT id, name, provider, provider_id, email, avatar_url, created_at FROM organizations WHERE provider = ? AND provider_id = ?",
		provider, providerID,
	).Scan(&o.ID, &o.Name, &o.Provider, &o.ProviderID, &email, &avatar, &o.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if email.Valid {
		o.Email = email.String
	}
	if avatar.Valid {
		o.AvatarURL = avatar.String
	}
	return &o, nil
}

func GetOrgByName(db *sql.DB, name string) (*model.Organization, error) {
	var o model.Organization
	var email, avatar sql.NullString
	err := db.QueryRow(
		"SELECT id, name, provider, provider_id, email, avatar_url, created_at FROM organizations WHERE name = ?",
		name,
	).Scan(&o.ID, &o.Name, &o.Provider, &o.ProviderID, &email, &avatar, &o.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if email.Valid {
		o.Email = email.String
	}
	if avatar.Valid {
		o.AvatarURL = avatar.String
	}
	return &o, nil
}

func GetOrgByID(db *sql.DB, id int64) (*model.Organization, error) {
	var o model.Organization
	var email, avatar sql.NullString
	err := db.QueryRow(
		"SELECT id, name, provider, provider_id, email, avatar_url, created_at FROM organizations WHERE id = ?",
		id,
	).Scan(&o.ID, &o.Name, &o.Provider, &o.ProviderID, &email, &avatar, &o.CreatedAt)
	if err != nil {
		return nil, err
	}
	if email.Valid {
		o.Email = email.String
	}
	if avatar.Valid {
		o.AvatarURL = avatar.String
	}
	return &o, nil
}

func ListOrgs(db *sql.DB, limit, offset int) ([]model.Organization, error) {
	rows, err := db.Query(
		"SELECT id, name, provider, provider_id, email, avatar_url, created_at FROM organizations ORDER BY name LIMIT ? OFFSET ?",
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var orgs []model.Organization
	for rows.Next() {
		var o model.Organization
		var email, avatar sql.NullString
		if err := rows.Scan(&o.ID, &o.Name, &o.Provider, &o.ProviderID, &email, &avatar, &o.CreatedAt); err != nil {
			return nil, err
		}
		if email.Valid {
			o.Email = email.String
		}
		if avatar.Valid {
			o.AvatarURL = avatar.String
		}
		orgs = append(orgs, o)
	}
	return orgs, nil
}

func GenerateUniqueOrgName(db *sql.DB, base string) (string, error) {
	name := base
	for i := 2; ; i++ {
		existing, err := GetOrgByName(db, name)
		if err != nil {
			return "", err
		}
		if existing == nil {
			return name, nil
		}
		name = fmt.Sprintf("%s%d", base, i)
	}
}

func GenerateKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}
	return "iroll_" + hex.EncodeToString(b), nil
}

func HashKey(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func CreateAPIKey(db *sql.DB, orgID int64, name string) (string, *model.APIKey, error) {
	raw, err := GenerateKey()
	if err != nil {
		return "", nil, fmt.Errorf("create api key: %w", err)
	}
	hash := HashKey(raw)
	now := model.NowISO()
	res, err := db.Exec(
		"INSERT INTO api_keys (org_id, key_hash, name, created_at) VALUES (?, ?, ?, ?)",
		orgID, hash, name, now,
	)
	if err != nil {
		return "", nil, fmt.Errorf("create api key: %w", err)
	}
	id, _ := res.LastInsertId()
	return raw, &model.APIKey{ID: id, OrgID: orgID, Name: name, CreatedAt: now}, nil
}

func Authenticate(db *sql.DB, rawKey string) (*model.Organization, error) {
	hash := HashKey(rawKey)
	var orgID int64
	var keyID int64
	err := db.QueryRow("SELECT id, org_id FROM api_keys WHERE key_hash = ?", hash).Scan(&keyID, &orgID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("invalid api key")
	}
	if err != nil {
		return nil, err
	}
	now := model.NowISO()
	db.Exec("UPDATE api_keys SET last_used_at = ? WHERE id = ?", now, keyID)
	return GetOrgByID(db, orgID)
}

func ListAPIKeys(db *sql.DB, orgID int64) ([]model.APIKey, error) {
	rows, err := db.Query(
		"SELECT id, org_id, name, last_used_at, created_at FROM api_keys WHERE org_id = ? ORDER BY created_at DESC",
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []model.APIKey
	for rows.Next() {
		var k model.APIKey
		var lastUsed sql.NullString
		if err := rows.Scan(&k.ID, &k.OrgID, &k.Name, &lastUsed, &k.CreatedAt); err != nil {
			return nil, err
		}
		if lastUsed.Valid {
			k.LastUsedAt = lastUsed.String
		}
		keys = append(keys, k)
	}
	return keys, nil
}

func DeleteAPIKey(db *sql.DB, keyID, orgID int64) error {
	res, err := db.Exec("DELETE FROM api_keys WHERE id = ? AND org_id = ?", keyID, orgID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("api key not found")
	}
	return nil
}

func CreatePackage(db *sql.DB, orgID int64, name, description, tags string) (*model.Package, error) {
	now := model.NowISO()
	res, err := db.Exec(
		"INSERT INTO packages (org_id, name, description, tags, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		orgID, name, description, tags, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create package: %w", err)
	}
	id, _ := res.LastInsertId()
	return &model.Package{ID: id, OrgID: orgID, Name: name, Description: description, Tags: tags, CreatedAt: now, UpdatedAt: now}, nil
}

func GetPackage(db *sql.DB, orgID int64, name string) (*model.Package, error) {
	var p model.Package
	err := db.QueryRow(
		"SELECT id, org_id, name, description, tags, downloads, created_at, updated_at FROM packages WHERE org_id = ? AND name = ?",
		orgID, name,
	).Scan(&p.ID, &p.OrgID, &p.Name, &p.Description, &p.Tags, &p.Downloads, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &p, err
}

func ListPackages(db *sql.DB, orgID int64, limit, offset int) ([]model.Package, error) {
	rows, err := db.Query(
		"SELECT id, org_id, name, description, tags, downloads, created_at, updated_at FROM packages WHERE org_id = ? ORDER BY name LIMIT ? OFFSET ?",
		orgID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pkgs []model.Package
	for rows.Next() {
		var p model.Package
		if err := rows.Scan(&p.ID, &p.OrgID, &p.Name, &p.Description, &p.Tags, &p.Downloads, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

func DeletePackage(db *sql.DB, orgID int64, name string) error {
	res, err := db.Exec("DELETE FROM packages WHERE org_id = ? AND name = ?", orgID, name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("package not found")
	}
	return nil
}

func IncrementDownloads(db *sql.DB, pkgID int64) {
	if _, err := db.Exec("UPDATE packages SET downloads = downloads + 1, updated_at = ? WHERE id = ?", model.NowISO(), pkgID); err != nil {
		log.Printf("increment downloads: %v", err)
	}
}

func CreateVersion(db *sql.DB, packageID int64, version, objectKey string, fileSize int64, checksum string) (*model.Version, error) {
	now := model.NowISO()
	res, err := db.Exec(
		"INSERT INTO versions (package_id, version, object_key, file_size, checksum, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		packageID, version, objectKey, fileSize, checksum, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, fmt.Errorf("version %s already exists", version)
		}
		return nil, fmt.Errorf("create version: %w", err)
	}
	id, _ := res.LastInsertId()
	return &model.Version{ID: id, PackageID: packageID, Version: version, ObjectKey: objectKey, FileSize: fileSize, Checksum: checksum, CreatedAt: now}, nil
}

func GetVersion(db *sql.DB, packageID int64, version string) (*model.Version, error) {
	var v model.Version
	err := db.QueryRow(
		"SELECT id, package_id, version, object_key, file_size, checksum, created_at FROM versions WHERE package_id = ? AND version = ?",
		packageID, version,
	).Scan(&v.ID, &v.PackageID, &v.Version, &v.ObjectKey, &v.FileSize, &v.Checksum, &v.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &v, err
}

func GetLatestVersion(db *sql.DB, packageID int64) (*model.Version, error) {
	var v model.Version
	err := db.QueryRow(
		"SELECT id, package_id, version, object_key, file_size, checksum, created_at FROM versions WHERE package_id = ? ORDER BY id DESC LIMIT 1",
		packageID,
	).Scan(&v.ID, &v.PackageID, &v.Version, &v.ObjectKey, &v.FileSize, &v.Checksum, &v.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &v, err
}

func ListVersions(db *sql.DB, packageID int64, limit, offset int) ([]model.Version, error) {
	rows, err := db.Query(
		"SELECT id, package_id, version, object_key, file_size, checksum, created_at FROM versions WHERE package_id = ? ORDER BY id DESC LIMIT ? OFFSET ?",
		packageID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var vers []model.Version
	for rows.Next() {
		var v model.Version
		if err := rows.Scan(&v.ID, &v.PackageID, &v.Version, &v.ObjectKey, &v.FileSize, &v.Checksum, &v.CreatedAt); err != nil {
			return nil, err
		}
		vers = append(vers, v)
	}
	return vers, nil
}

func DeleteVersionsByPackage(db *sql.DB, packageID int64) error {
	_, err := db.Exec("DELETE FROM versions WHERE package_id = ?", packageID)
	return err
}

func DeleteVersion(db *sql.DB, packageID int64, version string) error {
	res, err := db.Exec("DELETE FROM versions WHERE package_id = ? AND version = ?", packageID, version)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("version not found")
	}
	return nil
}

func ListVersionObjectKeys(db *sql.DB, packageID int64) ([]string, error) {
	rows, err := db.Query("SELECT object_key FROM versions WHERE package_id = ?", packageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, nil
}

// SearchPackages uses FTS5 virtual table with BM25 ranking.
func SearchPackages(db *sql.DB, query string, limit int) ([]model.Package, error) {
	rows, err := db.Query(`
		SELECT p.id, p.org_id, p.name, p.description, p.tags, p.downloads, p.created_at, p.updated_at,
		       o.name AS org_name
		FROM packages_fts f
		JOIN packages p ON p.id = f.rowid
		JOIN organizations o ON o.id = p.org_id
		WHERE packages_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pkgs []model.Package
	for rows.Next() {
		var p model.Package
		if err := rows.Scan(&p.ID, &p.OrgID, &p.Name, &p.Description, &p.Tags, &p.Downloads, &p.CreatedAt, &p.UpdatedAt, &p.OrgName); err != nil {
			return nil, err
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}
