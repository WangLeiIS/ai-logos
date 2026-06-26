package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
)

func TestNavigateGet(t *testing.T) {
	m := map[string]interface{}{
		"user_context": map[string]interface{}{"project": "blog"},
		"plain":        "hi",
	}
	if v, ok := navigateGet(m, "user_context.project"); !ok || v != "blog" {
		t.Fatalf("navigateGet nested = (%v, %v), want (blog, true)", v, ok)
	}
	if v, ok := navigateGet(m, "plain"); !ok || v != "hi" {
		t.Fatalf("navigateGet top = (%v, %v), want (hi, true)", v, ok)
	}
	if _, ok := navigateGet(m, "user_context.missing"); ok {
		t.Fatal("navigateGet missing key should return ok=false")
	}
	if _, ok := navigateGet(m, "plain.sub"); ok {
		t.Fatal("navigateGet into non-map should return ok=false")
	}
}

func TestNavigateSet(t *testing.T) {
	m := map[string]interface{}{}
	navigateSet(m, "user_context.project", "blog")
	if v, _ := navigateGet(m, "user_context.project"); v != "blog" {
		t.Fatalf("after set nested, got %v", v)
	}
	navigateSet(m, "user_context.project", "blog-v2")
	if v, _ := navigateGet(m, "user_context.project"); v != "blog-v2" {
		t.Fatalf("overwrite failed, got %v", v)
	}
	navigateSet(m, "a.b.c", 1)
	if v, _ := navigateGet(m, "a.b.c"); v != 1 {
		t.Fatalf("auto-create intermediate failed, got %v", v)
	}
}

func TestNavigateUnset(t *testing.T) {
	m := map[string]interface{}{
		"user_context": map[string]interface{}{"project": "blog", "todo": "x"},
	}
	if !navigateUnset(m, "user_context.todo") {
		t.Fatal("unset existing should return true")
	}
	if _, ok := navigateGet(m, "user_context.todo"); ok {
		t.Fatal("key still present after unset")
	}
	if v, _ := navigateGet(m, "user_context.project"); v != "blog" {
		t.Fatalf("sibling lost after unset, got %v", v)
	}
	if navigateUnset(m, "user_context.missing") {
		t.Fatal("unset missing should return false")
	}
}

func TestParseJSONOrText(t *testing.T) {
	cases := []struct {
		in   string
		want interface{}
	}{
		{"blog", "blog"},
		{"true", true},
		{"42", float64(42)},
		{`["a","b"]`, []interface{}{"a", "b"}},
		{`{"k":"v"}`, map[string]interface{}{"k": "v"}},
		{"not json at all", "not json at all"},
	}
	for _, c := range cases {
		got, err := parseJSONOrText(c.in)
		if err != nil {
			t.Fatalf("parseJSONOrText(%q) err: %v", c.in, err)
		}
		if !deepEqual(got, c.want) {
			t.Fatalf("parseJSONOrText(%q) = %#v, want %#v", c.in, got, c.want)
		}
	}
}

func deepEqual(a, b interface{}) bool {
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return string(aj) == string(bj)
}

// setupContextTestDB builds an outer+inner connection (schema only) and inserts
// one working page with a known raw context.
func setupContextTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	innerPath, outerPath := setupDualDB(t, dir)
	conn, err := OpenOuter(outerPath, innerPath)
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })

	ctx := `{"system_prompt":"hi","user_context":{"project":"blog","todo":["deploy"]}}`
	if _, err := conn.Exec(
		`INSERT INTO pages (page_id, cwd, context, created_at, updated_at) VALUES ('p1', '', ?, datetime('now'), datetime('now'))`,
		ctx,
	); err != nil {
		t.Fatal(err)
	}
	return conn
}

func TestSetContextKey(t *testing.T) {
	conn := setupContextTestDB(t)

	if err := SetContextKey(conn, "p1", "user_context.project", "blog-v2"); err != nil {
		t.Fatalf("SetContextKey: %v", err)
	}
	p, err := GetPageByPageID(conn, "p1")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal([]byte(p.Context), &got); err != nil {
		t.Fatal(err)
	}
	if v, _ := navigateGet(got, "user_context.project"); v != "blog-v2" {
		t.Fatalf("project not updated, got %v", v)
	}
	if v, _ := navigateGet(got, "system_prompt"); v != "hi" {
		t.Fatalf("system_prompt lost, got %v", v)
	}
	if _, ok := navigateGet(got, "user_context.todo"); !ok {
		t.Fatal("user_context.todo lost")
	}
}

func TestSetContextKeyNewNested(t *testing.T) {
	conn := setupContextTestDB(t)
	if err := SetContextKey(conn, "p1", "user_context.stats.count", "3"); err != nil {
		t.Fatalf("SetContextKey new nested: %v", err)
	}
	p, _ := GetPageByPageID(conn, "p1")
	var got map[string]interface{}
	json.Unmarshal([]byte(p.Context), &got)
	if v, _ := navigateGet(got, "user_context.stats.count"); v != float64(3) {
		t.Fatalf("new nested not created/parsed, got %v", v)
	}
}

func TestSetContextKeyStoresMarkerRaw(t *testing.T) {
	conn := setupContextTestDB(t)
	marker := `{"@sql":"SELECT value FROM inner.metadata WHERE key='name'"}`
	if err := SetContextKey(conn, "p1", "name", marker); err != nil {
		t.Fatalf("SetContextKey marker: %v", err)
	}
	p, _ := GetPageByPageID(conn, "p1")
	var got map[string]interface{}
	json.Unmarshal([]byte(p.Context), &got)
	val, _ := navigateGet(got, "name")
	obj, ok := val.(map[string]interface{})
	if !ok {
		t.Fatalf("marker not stored as object, got %#v", val)
	}
	if obj["@sql"] == nil {
		t.Fatalf("marker @sql lost, got %#v", obj)
	}
}

func TestUnsetContextKey(t *testing.T) {
	conn := setupContextTestDB(t)
	if err := UnsetContextKey(conn, "p1", "user_context.todo"); err != nil {
		t.Fatalf("UnsetContextKey: %v", err)
	}
	p, _ := GetPageByPageID(conn, "p1")
	var got map[string]interface{}
	json.Unmarshal([]byte(p.Context), &got)
	if _, ok := navigateGet(got, "user_context.todo"); ok {
		t.Fatal("key still present after unset")
	}
}

func TestUnsetContextKeyMissing(t *testing.T) {
	conn := setupContextTestDB(t)
	err := UnsetContextKey(conn, "p1", "does.not.exist")
	if !errors.Is(err, ErrContextKeyNotFound) {
		t.Fatalf("expected ErrContextKeyNotFound, got %v", err)
	}
}

func TestGetContextKey(t *testing.T) {
	conn := setupContextTestDB(t)
	v, err := GetContextKey(conn, "p1", "user_context.project", "")
	if err != nil {
		t.Fatalf("GetContextKey: %v", err)
	}
	if v != "blog" {
		t.Fatalf("GetContextKey = %v, want blog", v)
	}
}

func TestGetContextKeyMissing(t *testing.T) {
	conn := setupContextTestDB(t)
	_, err := GetContextKey(conn, "p1", "nope.nada", "")
	if !errors.Is(err, ErrContextKeyNotFound) {
		t.Fatalf("expected ErrContextKeyNotFound, got %v", err)
	}
}
