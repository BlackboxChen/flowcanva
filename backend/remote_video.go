package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	minRemoteVideoBytes = 10_000
	maxRemoteVideoBytes = int64(1 << 30)
)

// persistRemoteVideo makes local storage mandatory for generated video output.
// A remote URL is never returned to or persisted by a video node.
func persistRemoteVideo(ctx context.Context, rawURL string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("create video download request: %w", err)
	}
	response, err := llmClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("download generated video: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("download generated video: status %d", response.StatusCode)
	}

	extension, err := remoteVideoExtension(response.Header.Get("Content-Type"), rawURL)
	if err != nil {
		return "", err
	}
	dir := filepath.Join("uploads", "videos")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, "remote-video-*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	keep := false
	defer func() {
		_ = tmp.Close()
		if !keep {
			_ = os.Remove(tmpName)
		}
	}()

	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(response.Body, maxRemoteVideoBytes+1))
	if err != nil {
		return "", err
	}
	if written < minRemoteVideoBytes {
		return "", fmt.Errorf("generated video is too small: %d bytes", written)
	}
	if written > maxRemoteVideoBytes {
		return "", fmt.Errorf("generated video exceeds %d bytes", maxRemoteVideoBytes)
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	name := "video_" + hex.EncodeToString(hash.Sum(nil))[:20] + extension
	target := filepath.Join(dir, name)
	if _, err := os.Stat(target); err == nil {
		_ = os.Remove(tmpName)
		keep = true
		return "/uploads/videos/" + name, nil
	}
	if err := os.Rename(tmpName, target); err != nil {
		return "", err
	}
	keep = true
	return "/uploads/videos/" + name, nil
}

func remoteVideoExtension(contentType, rawURL string) (string, error) {
	mimeType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	extensions := map[string]string{
		"video/mp4": ".mp4", "video/webm": ".webm", "video/quicktime": ".mov",
		"video/x-matroska": ".mkv", "application/octet-stream": "",
	}
	if extension, ok := extensions[mimeType]; ok && extension != "" {
		return extension, nil
	}
	parsed, _ := url.Parse(rawURL)
	extension := strings.ToLower(filepath.Ext(parsed.Path))
	for _, allowed := range []string{".mp4", ".webm", ".mov", ".mkv"} {
		if extension == allowed {
			return extension, nil
		}
	}
	return "", fmt.Errorf("generated URL did not return a supported video: %s", contentType)
}
