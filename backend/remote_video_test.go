package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPersistRemoteVideo(t *testing.T) {
	payload := strings.Repeat("video-data", 2_000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	workdir := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workdir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	localURL, err := persistRemoteVideo(context.Background(), server.URL+"/result.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(localURL, "/uploads/videos/") || strings.Contains(localURL, server.URL) {
		t.Fatalf("unexpected local URL: %s", localURL)
	}
	path := filepath.Join(workdir, filepath.FromSlash(strings.TrimPrefix(localURL, "/")))
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != payload {
		t.Fatal("saved video content differs")
	}
}

func TestPersistRemoteVideoRejectsFailedDownload(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	if _, err := persistRemoteVideo(context.Background(), server.URL+"/missing.mp4"); err == nil {
		t.Fatal("expected failed download error")
	}
}
