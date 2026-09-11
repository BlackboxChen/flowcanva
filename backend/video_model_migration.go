package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

const zhengAPIBaseURL = "https://zhengapi.com"

var zhengAPIVideoModels = []map[string]any{
	{"name": "sd-2.0-15s-16x9-480p", "channel": "ZhengAPI", "base_url": zhengAPIBaseURL, "protocol": "zhengapi_video", "parameters": map[string]any{}},
	{"name": "sd-2.0-15s-9x16-480p", "channel": "ZhengAPI", "base_url": zhengAPIBaseURL, "protocol": "zhengapi_video", "parameters": map[string]any{}},
	{"name": "sd-2.0-sec-480p", "channel": "ZhengAPI", "base_url": zhengAPIBaseURL, "protocol": "zhengapi_video", "parameters": map[string]any{}},
}

func ensureZhengAPIVideoModels(db *sql.DB, log *slog.Logger) error {
	var raw string
	if err := db.QueryRow(`SELECT extra_config FROM node_configs WHERE node_type='video'`).Scan(&raw); err != nil {
		return err
	}
	merged, added, err := mergeZhengAPIVideoModels(raw)
	if err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE node_configs SET model_name='sd-2.0-15s-16x9-480p', api_channel='ZhengAPI', base_url='`+zhengAPIBaseURL+`', api_key='', extra_config=?, updated_at=datetime('now','localtime') WHERE node_type='video'`, merged); err != nil {
		return err
	}
	log.Info("zhengapi video models registered", "added", added)
	return nil
}

func mergeZhengAPIVideoModels(raw string) (string, int, error) {
	extra := map[string]any{}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &extra); err != nil {
			return "", 0, fmt.Errorf("parse video extra_config: %w", err)
		}
	}
	models, _ := extra["models"].([]any)
	// Rebuild ZhengAPI presets so removed 720p/1080p/fast models disappear from existing databases.
	filtered := make([]any, 0, len(models))
	for _, item := range models {
		model, ok := item.(map[string]any)
		if !ok {
			filtered = append(filtered, item)
			continue
		}
		name, _ := model["name"].(string)
		channel, _ := model["channel"].(string)
		if channel == "ZhengAPI" && (containsVideoSuffix(name, "720p") || containsVideoSuffix(name, "1080p") || strings.Contains(name, "sd-2.0fast-")) {
			continue
		}
		filtered = append(filtered, item)
	}
	models = filtered
	existing := map[string]bool{}
	for _, item := range models {
		if model, ok := item.(map[string]any); ok {
			name, _ := model["name"].(string)
			if model["channel"] == "ZhengAPI" && isZhengAPIVideoPreset(name) {
				model["base_url"] = zhengAPIBaseURL
				model["protocol"] = "zhengapi_video"
			}
			if name != "" {
				existing[name] = true
			}
		}
	}
	added := 0
	for _, preset := range zhengAPIVideoModels {
		name := preset["name"].(string)
		if !existing[name] {
			models = append(models, preset)
			added++
		}
	}
	extra["models"] = models
	merged, err := json.Marshal(extra)
	return string(merged), added, err
}

func isZhengAPIVideoPreset(name string) bool {
	for _, preset := range zhengAPIVideoModels {
		if preset["name"] == name {
			return true
		}
	}
	return false
}

func containsVideoSuffix(name, suffix string) bool {
	return len(name) >= len(suffix) && name[len(name)-len(suffix):] == suffix
}
