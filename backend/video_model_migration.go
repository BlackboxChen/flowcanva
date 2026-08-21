package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
)

var zhengAPIVideoModels = []map[string]any{
	{"name": "sd-2.0-15s-16x9-480p", "channel": "ZhengAPI", "base_url": "https://zhengapi.top", "protocol": "zhengapi_video", "parameters": map[string]any{}},
	{"name": "sd-2.0-15s-16x9-720p", "channel": "ZhengAPI", "base_url": "https://zhengapi.top", "protocol": "zhengapi_video", "parameters": map[string]any{}},
	{"name": "sd-2.0-15s-16x9-1080p", "channel": "ZhengAPI", "base_url": "https://zhengapi.top", "protocol": "zhengapi_video", "parameters": map[string]any{}},
	{"name": "sd-2.0-15s-9x16-480p", "channel": "ZhengAPI", "base_url": "https://zhengapi.top", "protocol": "zhengapi_video", "parameters": map[string]any{}},
	{"name": "sd-2.0-15s-9x16-720p", "channel": "ZhengAPI", "base_url": "https://zhengapi.top", "protocol": "zhengapi_video", "parameters": map[string]any{}},
	{"name": "sd-2.0-15s-9x16-1080p", "channel": "ZhengAPI", "base_url": "https://zhengapi.top", "protocol": "zhengapi_video", "parameters": map[string]any{}},
	{"name": "sd-2.0fast-15s-16x9-480p", "channel": "ZhengAPI", "base_url": "https://zhengapi.top", "protocol": "zhengapi_video", "parameters": map[string]any{}},
	{"name": "sd-2.0fast-15s-16x9-720p", "channel": "ZhengAPI", "base_url": "https://zhengapi.top", "protocol": "zhengapi_video", "parameters": map[string]any{}},
	{"name": "sd-2.0fast-15s-16x9-1080p", "channel": "ZhengAPI", "base_url": "https://zhengapi.top", "protocol": "zhengapi_video", "parameters": map[string]any{}},
	{"name": "sd-2.0fast-15s-9x16-480p", "channel": "ZhengAPI", "base_url": "https://zhengapi.top", "protocol": "zhengapi_video", "parameters": map[string]any{}},
	{"name": "sd-2.0fast-15s-9x16-720p", "channel": "ZhengAPI", "base_url": "https://zhengapi.top", "protocol": "zhengapi_video", "parameters": map[string]any{}},
	{"name": "sd-2.0fast-15s-9x16-1080p", "channel": "ZhengAPI", "base_url": "https://zhengapi.top", "protocol": "zhengapi_video", "parameters": map[string]any{}},
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
	if _, err := db.Exec(`UPDATE node_configs SET model_name='sd-2.0-15s-16x9-480p', api_channel='ZhengAPI', base_url='https://zhengapi.top', api_key='', extra_config=?, updated_at=datetime('now','localtime') WHERE node_type='video'`, merged); err != nil {
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
	existing := map[string]bool{}
	for _, item := range models {
		if model, ok := item.(map[string]any); ok {
			if name, _ := model["name"].(string); name != "" {
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
