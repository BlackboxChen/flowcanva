package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var embeddedImagePattern = regexp.MustCompile(`data:image/(png|jpeg|jpg|webp|gif);base64,[A-Za-z0-9+/=]+`)

// persistEmbeddedImage moves a Data URL out of JSON and into the uploads directory.
// The content-derived name also prevents the same image from being written repeatedly.
func persistEmbeddedImage(dataURL string) (string, error) {
	comma := strings.IndexByte(dataURL, ',')
	if comma < 0 {
		return "", fmt.Errorf("invalid image Data URL")
	}
	mimeType := strings.TrimSuffix(strings.TrimPrefix(dataURL[:comma], "data:image/"), ";base64")
	extensions := map[string]string{"png": ".png", "jpeg": ".jpg", "jpg": ".jpg", "webp": ".webp", "gif": ".gif"}
	extension, ok := extensions[strings.ToLower(mimeType)]
	if !ok {
		return "", fmt.Errorf("unsupported embedded image type: %s", mimeType)
	}

	hash := sha256.New()
	_, _ = io.WriteString(hash, dataURL[comma+1:])
	name := "generated_" + hex.EncodeToString(hash.Sum(nil))[:20] + extension
	dir := filepath.Join("uploads", "generated")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	target := filepath.Join(dir, name)
	if _, err := os.Stat(target); err == nil {
		return "/uploads/generated/" + name, nil
	}

	tmp, err := os.CreateTemp(dir, name+"-*.tmp")
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
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(dataURL[comma+1:]))
	if _, err := io.Copy(tmp, decoder); err != nil {
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, target); err != nil {
		return "", err
	}
	keep = true
	return "/uploads/generated/" + name, nil
}

func replaceEmbeddedImages(value string) (string, int, error) {
	count := 0
	var firstErr error
	replaced := embeddedImagePattern.ReplaceAllStringFunc(value, func(match string) string {
		url, err := persistEmbeddedImage(match)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return match
		}
		count++
		return url
	})
	return replaced, count, firstErr
}

// migrateEmbeddedNodeMedia keeps legacy canvases from loading megabytes of base64 per node.
func migrateEmbeddedNodeMedia(db *sql.DB, log *slog.Logger) error {
	rows, err := db.Query(`SELECT id, content FROM nodes WHERE content LIKE '%data:image/%;base64,%'`)
	if err != nil {
		return err
	}
	type update struct{ id, content string }
	updates := make([]update, 0)
	total := 0
	for rows.Next() {
		var id, content string
		if err := rows.Scan(&id, &content); err != nil {
			rows.Close()
			return err
		}
		replaced, count, replaceErr := replaceEmbeddedImages(content)
		if replaceErr != nil {
			log.Warn("embedded image migration skipped value", "node_id", id, "error", replaceErr)
		}
		if count > 0 {
			updates = append(updates, update{id: id, content: replaced})
			total += count
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range updates {
		if _, err := db.Exec(`UPDATE nodes SET content = ? WHERE id = ?`, item.content, item.id); err != nil {
			return err
		}
	}
	if total > 0 {
		log.Info("embedded node images migrated", "images", total, "nodes", len(updates))
	}
	return nil
}

// materializeImageResponse replaces upstream image payloads with local upload
// paths before they can be persisted inside a canvas node by the frontend.
func materializeImageResponse(body []byte) ([]byte, error) {
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return body, err
	}
	changed, err := materializeResponseValue(payload)
	if err != nil || !changed {
		return body, err
	}
	return json.Marshal(payload)
}

func materializeResponseValue(value any) (bool, error) {
	changed := false
	switch current := value.(type) {
	case []any:
		for _, child := range current {
			childChanged, err := materializeResponseValue(child)
			if err != nil {
				return changed, err
			}
			changed = changed || childChanged
		}
	case map[string]any:
		if raw, ok := current["b64_json"].(string); ok && raw != "" {
			if !strings.HasPrefix(raw, "data:") {
				raw = "data:image/png;base64," + raw
			}
			url, err := persistEmbeddedImage(raw)
			if err != nil {
				return changed, err
			}
			current["url"] = url
			delete(current, "b64_json")
			changed = true
		}
		if raw, ok := current["url"].(string); ok && strings.HasPrefix(raw, "data:image/") {
			url, err := persistEmbeddedImage(raw)
			if err != nil {
				return changed, err
			}
			current["url"] = url
			changed = true
		} else if ok && isRemoteHTTPURL(raw) {
			url, err := persistRemoteImage(raw)
			if err != nil {
				return changed, err
			}
			current["url"] = url
			changed = true
		}
		for _, child := range current {
			childChanged, err := materializeResponseValue(child)
			if err != nil {
				return changed, err
			}
			changed = changed || childChanged
		}
	}
	return changed, nil
}
