package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCollaborationBackupCanRestoreDatabaseAndMedia(t *testing.T) {
	// No parallel tests: media paths intentionally follow the server working directory.
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err = os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	h, _ := collaborationTestHandler(t)
	if err = os.MkdirAll("uploads", 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile("uploads/reference.png", []byte("test reference"), 0600); err != nil {
		t.Fatal(err)
	}
	testNode(t, h, "saved", "image", `{"url":"/uploads/reference.png"}`)
	backup, err := h.createBackup()
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]string
	raw, err := os.ReadFile(filepath.Join(backup, "media.json"))
	if err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(raw, &manifest) != nil || len(manifest) != 1 {
		t.Fatal("manifest invalid")
	}
	restored, err := initDB(filepath.Join(backup, "data.db"), h.log)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var content string
	if err = restored.QueryRow(`SELECT content FROM nodes WHERE id='saved'`).Scan(&content); err != nil {
		t.Fatal(err)
	}
	image, err := os.ReadFile(filepath.Join("backups", "media", manifest["reference.png"]))
	if err != nil || string(image) != "test reference" {
		t.Fatal("media restoration failed")
	}
}
