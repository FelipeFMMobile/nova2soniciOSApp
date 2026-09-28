package integrations

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInvalidProfilesFailClosed(t *testing.T) {
	for _, data := range []string{`{"tools":{"x":{"policy":"unsafe"}}}`, `{"tools":{"x":{"policy":"confirm_later"}}}`, `{"tools":{"x":{"policy":"read_only","direct":true}}}`, `{"tools":{"x":{"policy":"explicit_intent"}}}`, `{"tools":{"x":{"policy":"read_only","secret":"literal"}}}`, `{"tools":{"notes.delete":{"policy":"read_only"}}}`} {
		t.Run(data, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("STS_INTEGRATIONS_DIR", dir)
			if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(); err == nil {
				t.Fatal("invalid profile accepted")
			}
		})
	}
}
func TestBuiltInPoliciesCannotBeDowngraded(t *testing.T) {
	if ValidatePolicy("notes.delete", "read_only") == nil {
		t.Fatal("destructive policy downgraded")
	}
}
