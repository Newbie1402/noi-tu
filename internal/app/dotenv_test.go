package app

import (
	"os"
	"path/filepath"
	"testing"

	"noitu/internal/config"
)

func TestLoadDotEnvPreservesExistingEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("TEST_DOTENV_QUOTED='hello world'\nTEST_DOTENV_EXISTING=from-file\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_DOTENV_EXISTING", "from-environment")
	config.LoadDotEnv(path)
	if got := os.Getenv("TEST_DOTENV_QUOTED"); got != "hello world" {
		t.Fatalf("quoted value: %q", got)
	}
	if got := os.Getenv("TEST_DOTENV_EXISTING"); got != "from-environment" {
		t.Fatalf("existing value overwritten: %q", got)
	}
}
