package cmd

import (
	"bytes"
	"testing"
)

func TestVersionDefaultIsDev(t *testing.T) {
	if Version != "dev" {
		t.Errorf("Version = %q, want %q", Version, "dev")
	}
}

func TestVersionCommandRegistered(t *testing.T) {
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Name() == "version" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("version command not registered under rootCmd")
	}
}

func TestVersionCommandOutput(t *testing.T) {
	old := Version
	defer func() { Version = old }()
	Version = "1.2.3"

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"version"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("version command failed: %v", err)
	}
	if got := buf.String(); got != "1.2.3\n" {
		t.Errorf("output = %q, want %q", got, "1.2.3\n")
	}
}

