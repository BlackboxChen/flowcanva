package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var workflowMediaLocks sync.Map

func initWorkflowPersistence(db *sql.DB) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS workflow_media (source_url TEXT PRIMARY KEY, local_url TEXT NOT NULL, created_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS workflow_jobs (id TEXT PRIMARY KEY, server_url TEXT NOT NULL, prompt_id TEXT NOT NULL, canvas_id TEXT NOT NULL DEFAULT '', node_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, remote_json TEXT NOT NULL DEFAULT '[]', local_json TEXT NOT NULL DEFAULT '[]', error TEXT NOT NULL DEFAULT '', attempts INTEGER NOT NULL DEFAULT 0, next_retry INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL, UNIQUE(server_url,prompt_id))`,
	} {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func localWorkflowFile(raw string) (string, error) {
	if !strings.HasPrefix(raw, "/uploads/") {
		return "", fmt.Errorf("媒体地址必须为本地 /uploads/ 地址")
	}
	decoded, err := url.PathUnescape(strings.TrimPrefix(raw, "/uploads/"))
	if err != nil {
		return "", err
	}
	root, _ := filepath.Abs("uploads")
	path, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(decoded)))
	if err != nil || !strings.HasPrefix(strings.ToLower(path), strings.ToLower(root+string(os.PathSeparator))) {
		return "", fmt.Errorf("无效媒体路径")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() || info.Size() == 0 {
		return "", fmt.Errorf("本地媒体文件为空")
	}
	return path, nil
}

func (h *Handler) persistWorkflowURL(ctx context.Context, source string) (string, error) {
	if !isRemoteHTTPURL(source) {
		_, err := localWorkflowFile(source)
		return source, err
	}
	lockValue, _ := workflowMediaLocks.LoadOrStore(source, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	var existing string
	if err := h.db.QueryRow(`SELECT local_url FROM workflow_media WHERE source_url=?`, source).Scan(&existing); err == nil {
		if _, err = localWorkflowFile(existing); err == nil {
			return existing, nil
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return "", err
	}
	response, err := (&http.Client{Timeout: 3 * time.Minute}).Do(req)
	if err != nil {
		return "", fmt.Errorf("下载媒体失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return "", fmt.Errorf("下载媒体 HTTP %d", response.StatusCode)
	}
	ct := strings.ToLower(strings.Split(response.Header.Get("Content-Type"), ";")[0])
	extensions := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp", "image/gif": ".gif", "image/avif": ".avif", "video/mp4": ".mp4", "video/webm": ".webm", "video/quicktime": ".mov", "video/x-matroska": ".mkv", "audio/wav": ".wav", "audio/x-wav": ".wav", "audio/mpeg": ".mp3", "audio/flac": ".flac", "audio/ogg": ".ogg", "audio/mp4": ".m4a"}
	ext := extensions[ct]
	if ext == "" && ct == "application/octet-stream" {
		parsed, _ := url.Parse(source)
		name := parsed.Query().Get("filename")
		if name == "" {
			name = parsed.Path
		}
		candidate := strings.ToLower(filepath.Ext(name))
		for _, allowed := range extensions {
			if candidate == allowed {
				ext = candidate
				break
			}
		}
	}
	if ext == "" {
		return "", fmt.Errorf("上游未返回支持的媒体文件 (%s)", ct)
	}
	dir := filepath.Join("uploads", "workflow")
	if err = os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, "download-*.tmp")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(response.Body, (1<<30)+1))
	if err != nil {
		return "", err
	}
	if n == 0 || n > 1<<30 {
		return "", fmt.Errorf("媒体文件为空或超过1GB")
	}
	if err = tmp.Sync(); err != nil {
		return "", err
	}
	if err = tmp.Close(); err != nil {
		return "", err
	}
	filename := hex.EncodeToString(hash.Sum(nil)) + ext
	target := filepath.Join(dir, filename)
	if _, err = os.Stat(target); os.IsNotExist(err) {
		if err = os.Rename(tmp.Name(), target); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	local := "/uploads/workflow/" + filename
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO workflow_media(source_url,local_url,created_at) VALUES(?,?,?) ON CONFLICT(source_url) DO UPDATE SET local_url=excluded.local_url`, source, local, time.Now().Unix())
	if err != nil {
		return "", err
	}
	category := "其他"
	if strings.HasPrefix(ct, "video/") {
		category = "视频"
	}
	if strings.HasPrefix(ct, "audio/") {
		category = "音频"
	}
	if _, err = tx.Exec(`INSERT OR IGNORE INTO assets(id,filename,url,size,category,tags) VALUES(?,?,?,?,?,?)`, "wf_"+filename, filename, local, n, category, `["ComfyUI","本地副本"]`); err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	h.log.Info("workflow media persisted", "bytes", n, "local_url", local)
	return local, nil
}

// Used by ALL node writes, including old clients and external scripts.
func (h *Handler) localizeNodeContent(ctx context.Context, kind, content string) (string, error) {
	if kind != "workflow" && kind != "image" && kind != "video" && kind != "asset" {
		return content, nil
	}
	var data map[string]any
	if json.Unmarshal([]byte(content), &data) != nil {
		return content, nil
	}
	fields := map[string]string{"workflow": "outputs", "image": "generated_images", "video": "videos"}
	var entries []any
	if kind == "asset" {
		entries = []any{data}
	} else {
		entries, _ = data[fields[kind]].([]any)
	}
	changed := false
	for index, entry := range entries {
		if source, ok := entry.(string); ok && isRemoteHTTPURL(source) {
			local, err := h.persistWorkflowURL(ctx, source)
			if err != nil {
				return "", err
			}
			entries[index] = local
			changed = true
			continue
		}
		item, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if kind == "workflow" && item["type"] == "text" {
			continue
		}
		key := "url"
		if kind == "workflow" {
			key = "value"
		}
		source, _ := item[key].(string)
		if !isRemoteHTTPURL(source) {
			continue
		}
		local, err := h.persistWorkflowURL(ctx, source)
		if err != nil {
			return "", fmt.Errorf("生成结果尚未保存到本地，已保留原节点: %w", err)
		}
		item[key] = local
		if item["sourceURL"] == nil {
			item["sourceURL"] = source
		}
		changed = true
	}
	if kind == "asset" {
		if source, ok := data["dataUrl"].(string); ok && isRemoteHTTPURL(source) {
			local, err := h.persistWorkflowURL(ctx, source)
			if err != nil {
				return "", err
			}
			data["dataUrl"] = local
			data["sourceURL"] = source
			changed = true
		}
	}
	if !changed {
		return content, nil
	}
	encoded, err := json.Marshal(data)
	return string(encoded), err
}
