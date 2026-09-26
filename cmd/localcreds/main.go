package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	created, password, err := ensureCredentials("config/.env")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if created {
		fmt.Printf("Created local admin credentials in config/.env\nUsername: admin\nPassword: %s\n", password)
	} else {
		fmt.Println("Using local admin credentials in config/.env")
	}
}

func ensureCredentials(path string) (bool, string, error) {
	if _, err := os.Stat(path); err == nil {
		return false, "", nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return false, "", err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return false, "", err
	}
	password := base64.RawURLEncoding.EncodeToString(secret)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	contents := fmt.Sprintf("ENGLANDSOFTWARE_ADMIN_USERNAME=admin\nENGLANDSOFTWARE_ADMIN_PASSWORD=%s\nENGLANDSOFTWARE_SECURE_COOKIES=false\n", password)
	if _, err := f.WriteString(contents); err != nil {
		f.Close()
		os.Remove(path)
		return false, "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return false, "", err
	}
	return true, password, nil
}
