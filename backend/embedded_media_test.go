package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaterializeImageResponse(t *testing.T) {
	workdir := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workdir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	body := []byte(`{"data":[{"b64_json":"aGVsbG8="}]}`)
	result, err := materializeImageResponse(body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(result), "b64_json") || strings.Contains(string(result), "aGVsbG8=") {
		t.Fatalf("base64 remained in response: %s", result)
	}
	var decoded struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Data) != 1 || !strings.HasPrefix(decoded.Data[0].URL, "/uploads/generated/") {
		t.Fatalf("unexpected materialized response: %s", result)
	}
	path := filepath.Join(workdir, filepath.FromSlash(strings.TrimPrefix(decoded.Data[0].URL, "/")))
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hello" {
		t.Fatalf("saved content = %q", content)
	}
}

func TestReplaceEmbeddedImagesInsideHTML(t *testing.T) {
	workdir := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workdir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	value := `<span><img src="data:image/png;base64,aGVsbG8=" /></span>`
	replaced, count, err := replaceEmbeddedImages(value)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || strings.Contains(replaced, "base64") || !strings.Contains(replaced, "/uploads/generated/") {
		t.Fatalf("unexpected replacement: count=%d value=%s", count, replaced)
	}
}
