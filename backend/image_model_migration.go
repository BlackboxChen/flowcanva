package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

var geekNowImageModels = []map[string]any{
	{"name": "gpt-image-2", "channel": "GeekNow", "base_url": "https://geeknow.ai/v1", "api_key": "", "protocol": "geeknow", "parameters": map[string]any{"supports_img2img": true}},
	{"name": "gpt-image-2-pro", "channel": "GeekNow", "base_url": "https://geeknow.ai/v1", "api_key": "", "protocol": "geeknow", "parameters": map[string]any{"supports_img2img": true}},
	{"name": "gpt-image-2.5-flare", "channel": "GeekNow", "base_url": "https://geeknow.ai/v1", "api_key": "", "protocol": "geeknow", "parameters": map[string]any{"supports_img2img": true}},
	{"name": "gpt-image-2.5-sunburst", "channel": "GeekNow", "base_url": "https://geeknow.ai/v1", "api_key": "", "protocol": "geeknow", "parameters": map[string]any{"supports_img2img": true}},
	{"name": "doubao-seedream-4-0-250828", "channel": "GeekNow", "base_url": "https://geeknow.ai/v1", "api_key": "", "protocol": "geeknow", "parameters": map[string]any{"supports_img2img": true}},
	{"name": "doubao-seedream-4-5-251128", "channel": "GeekNow", "base_url": "https://geeknow.ai/v1", "api_key": "", "protocol": "geeknow", "parameters": map[string]any{"supports_img2img": true}},
	{"name": "doubao-seedream-5-0-260128", "channel": "GeekNow", "base_url": "https://geeknow.ai/v1", "api_key": "", "protocol": "geeknow", "parameters": map[string]any{"supports_img2img": true}},
}

func ensureGeekNowImageModels(db *sql.DB, log *slog.Logger) error {
	var raw string
	if err := db.QueryRow(`SELECT extra_config FROM node_configs WHERE node_type='image'`).Scan(&raw); err != nil {
		return err
	}
	merged, added, removed, err := mergeGeekNowImageModels(raw)
	if err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE node_configs SET model_name='gpt-image-2', api_channel='GeekNow', base_url='https://geeknow.ai/v1', api_key='', extra_config=?, updated_at=datetime('now','localtime') WHERE node_type='image'`, merged); err != nil {
		return err
	}
	log.Info("geeknow image models registered", "added", added, "removed_zhengapi", removed)
	return nil
}

func mergeGeekNowImageModels(raw string) (string, int, int, error) {
	extra := map[string]any{}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &extra); err != nil {
			return "", 0, 0, fmt.Errorf("parse image extra_config: %w", err)
		}
	}
	models, _ := extra["models"].([]any)
	kept := make([]any, 0, len(models)+len(geekNowImageModels))
	existing := map[string]bool{}
	geekNowNames := map[string]bool{}
	for _, preset := range geekNowImageModels {
		geekNowNames[preset["name"].(string)] = true
	}
	retiredNames := map[string]bool{"gpt-image-2-vip": true, "grok-4-2-image": true}
	removed := 0
	for _, item := range models {
		if model, ok := item.(map[string]any); ok {
			name, _ := model["name"].(string)
			channel, _ := model["channel"].(string)
			if channel == "GeekNow" {
				if baseURL, ok := model["base_url"].(string); ok {
					baseURL = strings.ReplaceAll(baseURL, "https://www.geeknow.top", "https://geeknow.ai")
					baseURL = strings.ReplaceAll(baseURL, "https://www.geeknow.ai", "https://geeknow.ai")
					model["base_url"] = baseURL
				}
			}
			if channel == "ZhengAPI" || retiredNames[name] || (geekNowNames[name] && channel != "GeekNow") {
				removed++
				continue
			}
			if name != "" {
				existing[name] = true
			}
		}
		kept = append(kept, item)
	}
	added := 0
	for _, preset := range geekNowImageModels {
		name := preset["name"].(string)
		if existing[name] {
			continue
		}
		kept = append(kept, preset)
		added++
	}
	extra["models"] = kept
	merged, err := json.Marshal(extra)
	return string(merged), added, removed, err
}
