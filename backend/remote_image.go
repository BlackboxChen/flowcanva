package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxRemoteImageBytes = 128 << 20

var remoteImageClient = &http.Client{Timeout: 2 * time.Minute}

func isRemoteHTTPURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

// persistRemoteImage downloads an upstream image while it is still available.
// Nodes then reference only the local /uploads/generated path.
func persistRemoteImage(rawURL string) (string, error) {
	response, err := remoteImageClient.Get(rawURL)
	if err != nil {
		return "", fmt.Errorf("download generated image: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("download generated image: status %d", response.StatusCode)
	}

	extension, err := remoteImageExtension(response.Header.Get("Content-Type"), rawURL)
	if err != nil {
		return "", err
	}
	dir := filepath.Join("uploads", "generated")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, "remote-image-*.tmp")
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
	reader := io.LimitReader(response.Body, maxRemoteImageBytes+1)
	written, err := io.Copy(io.MultiWriter(tmp, hash), reader)
	if err != nil {
		return "", err
	}
	if written > maxRemoteImageBytes {
		return "", fmt.Errorf("generated image exceeds %d bytes", maxRemoteImageBytes)
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	name := "generated_" + hex.EncodeToString(hash.Sum(nil))[:20] + extension
	target := filepath.Join(dir, name)
	if _, err := os.Stat(target); err == nil {
		_ = os.Remove(tmpName)
		keep = true
		return "/uploads/generated/" + name, nil
	}
	if err := os.Rename(tmpName, target); err != nil {
		return "", err
	}
	keep = true
	return "/uploads/generated/" + name, nil
}

func remoteImageExtension(contentType, rawURL string) (string, error) {
	mimeType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	extensions := map[string]string{
		"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp",
		"image/gif": ".gif", "image/avif": ".avif",
	}
	if extension, ok := extensions[mimeType]; ok {
		return extension, nil
	}
	parsed, _ := url.Parse(rawURL)
	extension := strings.ToLower(filepath.Ext(parsed.Path))
	if extension == ".jpeg" {
		extension = ".jpg"
	}
	for _, allowed := range extensions {
		if extension == allowed {
			return extension, nil
		}
	}
	return "", fmt.Errorf("generated URL did not return a supported image: %s", contentType)
}
