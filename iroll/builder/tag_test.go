package builder

import "testing"

func TestParseTagDefaultLatest(t *testing.T) {
	name, version, err := ParseTag("my-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "my-agent" || version != "latest" {
		t.Errorf("got %q:%q, want my-agent:latest", name, version)
	}
}

func TestParseTagWithVersion(t *testing.T) {
	name, version, err := ParseTag("my-agent:v0.1.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "my-agent" || version != "v0.1.0" {
		t.Errorf("got %q:%q, want my-agent:v0.1.0", name, version)
	}
}

func TestParseTagEmpty(t *testing.T) {
	_, _, err := ParseTag("")
	if err == nil {
		t.Fatal("expected error for empty tag")
	}
}

func TestParseTagEmptyName(t *testing.T) {
	_, _, err := ParseTag(":v1")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestParseTagEmptyVersion(t *testing.T) {
	_, _, err := ParseTag("name:")
	if err == nil {
		t.Fatal("expected error for empty version")
	}
}

func TestParseTagInvalidName(t *testing.T) {
	_, _, err := ParseTag("../escape:v1")
	if err == nil {
		t.Fatal("expected error for invalid name")
	}
}
