package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
)

var zhengAPIImageModels = []map[string]any{
	{"name": "grok-imagine-1.0", "channel": "ZhengAPI", "base_url": "https://zhengapi.top/v1", "api_key": "", "protocol": "zhengapi-grok", "parameters": map[string]any{}},
	{"name": "grok-imagine-1.0-edit", "channel": "ZhengAPI", "base_url": "https://zhengapi.top/v1", "api_key": "", "protocol": "zhengapi-grok", "parameters": map[string]any{"supports_img2img": true}},
	{"name": "firefly-nano-banana", "channel": "ZhengAPI", "base_url": "https://zhengapi.top/v1", "api_key": "", "protocol": "zhengapi-firefly", "parameters": map[string]any{"supports_img2img": true}},
	{"name": "firefly-nano-banana-pro", "channel": "ZhengAPI", "base_url": "https://zhengapi.top/v1", "api_key": "", "protocol": "zhengapi-firefly", "parameters": map[string]any{"supports_img2img": true}},
	{"name": "firefly-nano-banana2", "channel": "ZhengAPI", "base_url": "https://zhengapi.top/v1", "api_key": "", "protocol": "zhengapi-firefly", "parameters": map[string]any{"supports_img2img": true}},
	{"name": "firefly-gpt-image", "channel": "ZhengAPI", "base_url": "https://zhengapi.top/v1", "api_key": "", "protocol": "zhengapi-firefly", "parameters": map[string]any{"supports_img2img": true}},
}

func ensureZhengAPIImageModels(db *sql.DB, log *slog.Logger) error {
	var raw string
	if err := db.QueryRow(`SELECT extra_config FROM node_configs WHERE node_type='image'`).Scan(&raw); err != nil {
		return err
	}
	merged, added, err := mergeZhengAPIImageModels(raw)
	if err != nil {
		return err
	}
	if added == 0 {
		return nil
	}
	if _, err := db.Exec(`UPDATE node_configs SET extra_config=?, updated_at=datetime('now','localtime') WHERE node_type='image'`, merged); err != nil {
		return err
	}
	log.Info("zhengapi image models registered", "added", added)
	return nil
}

func mergeZhengAPIImageModels(raw string) (string, int, error) {
	extra := map[string]any{}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &extra); err != nil {
			return "", 0, fmt.Errorf("parse image extra_config: %w", err)
		}
	}
	models, _ := extra["models"].([]any)
	existing := map[string]bool{}
	for _, item := range models {
		if model, ok := item.(map[string]any); ok {
			if name, ok := model["name"].(string); ok {
				existing[name] = true
			}
		}
	}
	added := 0
	for _, preset := range zhengAPIImageModels {
		name := preset["name"].(string)
		if existing[name] {
			continue
		}
		models = append(models, preset)
		added++
	}
	extra["models"] = models
	merged, err := json.Marshal(extra)
	return string(merged), added, err
}
