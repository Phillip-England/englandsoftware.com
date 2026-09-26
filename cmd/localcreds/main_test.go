package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureCredentialsCreatesAndReusesPrivateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", ".env")
	created, password, err := ensureCredentials(path)
	if err != nil || !created || password == "" {
		t.Fatalf("first run: created=%t password length=%d err=%v", created, len(password), err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("credentials permissions: %o", info.Mode().Perm())
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "ENGLANDSOFTWARE_ADMIN_PASSWORD="+password+"\n") {
		t.Fatal("generated password missing from credentials file")
	}
	created, nextPassword, err := ensureCredentials(path)
	if err != nil || created || nextPassword != "" {
		t.Fatalf("second run: created=%t password=%q err=%v", created, nextPassword, err)
	}
	again, err := os.ReadFile(path)
	if err != nil || string(again) != string(contents) {
		t.Fatalf("credentials changed on second run: %v", err)
	}
}
