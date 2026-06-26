package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrContextKeyNotFound is returned when a context path does not exist.
var ErrContextKeyNotFound = errors.New("context key not found")

// parseJSONOrText parses s as JSON if it is valid; otherwise returns s as a plain string.
func parseJSONOrText(s string) (interface{}, error) {
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err == nil {
		return v, nil
	}
	return s, nil
}

// navigateGet walks a dot-separated path through nested map[string]interface{} values.
// Returns (value, true) if the full path exists, (nil, false) otherwise.
func navigateGet(m map[string]interface{}, path string) (interface{}, bool) {
	parts := strings.Split(path, ".")
	var cur interface{} = m
	for _, part := range parts {
		obj, ok := cur.(map[string]interface{})
		if !ok {
			return nil, false
		}
		val, exists := obj[part]
		if !exists {
			return nil, false
		}
		cur = val
	}
	return cur, true
}

// navigateSet walks a dot-separated path, creating intermediate maps as needed,
// and sets the leaf to value.
func navigateSet(m map[string]interface{}, path string, value interface{}) {
	parts := strings.Split(path, ".")
	cur := m
	for i, part := range parts {
		if i == len(parts)-1 {
			cur[part] = value
			return
		}
		next, ok := cur[part].(map[string]interface{})
		if !ok {
			next = map[string]interface{}{}
			cur[part] = next
		}
		cur = next
	}
}

// navigateUnset removes the leaf at path. Returns true if it existed.
func navigateUnset(m map[string]interface{}, path string) bool {
	parts := strings.Split(path, ".")
	cur := m
	for i, part := range parts {
		if i == len(parts)-1 {
			if _, exists := cur[part]; !exists {
				return false
			}
			delete(cur, part)
			return true
		}
		next, ok := cur[part].(map[string]interface{})
		if !ok {
			return false
		}
		cur = next
	}
	return false
}

// GetContextKey resolves the page's full context (ResolveContext, including @file/@sql
// resolution and loop injection) then navigates to path. irollPath is the iroll package
// root directory, used only to resolve @file markers.
func GetContextKey(db *sql.DB, pageID, path, irollPath string) (interface{}, error) {
	p, err := GetPageByPageID(db, pageID)
	if err != nil {
		return nil, err
	}
	resolved, err := ResolveContext(p.Context, irollPath, db, pageID)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(resolved), &m); err != nil {
		return nil, fmt.Errorf("parse resolved context: %w", err)
	}
	val, found := navigateGet(m, path)
	if !found {
		return nil, fmt.Errorf("context key %q: %w", path, ErrContextKeyNotFound)
	}
	return val, nil
}

// SetContextKey parses rawValue (json-or-text), then reads-modifies-writes the page's
// raw context, setting the leaf at path. @file/@sql markers on other keys are preserved.
func SetContextKey(db *sql.DB, pageID, path, rawValue string) error {
	p, err := GetPageByPageID(db, pageID)
	if err != nil {
		return err
	}
	m := map[string]interface{}{}
	if strings.TrimSpace(p.Context) != "" && p.Context != "null" {
		if err := json.Unmarshal([]byte(p.Context), &m); err != nil {
			return fmt.Errorf("parse page context as JSON: %w", err)
		}
	}
	value, err := parseJSONOrText(rawValue)
	if err != nil {
		return err
	}
	navigateSet(m, path, value)
	out, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal context: %w", err)
	}
	_, err = db.Exec("UPDATE pages SET context = ?, updated_at = ? WHERE page_id = ?", string(out), nowISO(), pageID)
	return err
}

// UnsetContextKey reads-modifies-writes the page's raw context, removing the leaf at path.
func UnsetContextKey(db *sql.DB, pageID, path string) error {
	p, err := GetPageByPageID(db, pageID)
	if err != nil {
		return err
	}
	m := map[string]interface{}{}
	if strings.TrimSpace(p.Context) != "" && p.Context != "null" {
		if err := json.Unmarshal([]byte(p.Context), &m); err != nil {
			return fmt.Errorf("parse page context as JSON: %w", err)
		}
	}
	if !navigateUnset(m, path) {
		return fmt.Errorf("context key %q: %w", path, ErrContextKeyNotFound)
	}
	out, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal context: %w", err)
	}
	_, err = db.Exec("UPDATE pages SET context = ?, updated_at = ? WHERE page_id = ?", string(out), nowISO(), pageID)
	return err
}
