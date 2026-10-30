package home

import (
	"path/filepath"
	"testing"
)

func TestDir(t *testing.T) {
	t.Setenv(EnvVar, "custom")
	if d, _ := Dir(); d != "custom" {
		t.Fatalf("Dir = %q", d)
	}

	t.Setenv(EnvVar, "")
	t.Setenv("HOME", "/h")
	t.Setenv("USERPROFILE", "/h")
	if d, _ := Dir(); d != filepath.Join("/h", ".dnd") {
		t.Fatalf("Dir = %q", d)
	}
}
