package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPromptRejectsMissingAndEmptyFiles(t *testing.T) {
	if _, err := LoadPrompt(filepath.Join(t.TempDir(), "missing.md")); err == nil {
		t.Fatal("missing prompt accepted")
	}
	empty := filepath.Join(t.TempDir(), "empty.md")
	if err := os.WriteFile(empty, []byte(" \n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPrompt(empty); err == nil {
		t.Fatal("empty prompt accepted")
	}
}

func TestLoadPromptTrimsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "system.md")
	if err := os.WriteFile(path, []byte("  instrução segura\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadPrompt(path)
	if err != nil || got != "instrução segura" {
		t.Fatalf("got %q, %v", got, err)
	}
}
