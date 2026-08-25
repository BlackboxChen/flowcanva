package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
)

var megaBYVideoModels = []map[string]any{
	{"name": "megaby-videos-standard", "channel": "MegaBY", "base_url": "https://newapi.megabyai.cc", "protocol": "megaby_video", "parameters": map[string]any{}},
	{"name": "megaby-videos-fast", "channel": "MegaBY", "base_url": "https://newapi.megabyai.cc", "protocol": "megaby_video", "parameters": map[string]any{}},
	{"name": "megaby-videos-mini", "channel": "MegaBY", "base_url": "https://newapi.megabyai.cc", "protocol": "megaby_video", "parameters": map[string]any{}},
}

func ensureMegaBYVideoModels(db *sql.DB, log *slog.Logger) error {
	var raw string
	if err := db.QueryRow(`SELECT extra_config FROM node_configs WHERE node_type='video'`).Scan(&raw); err != nil {
		return err
	}
	merged, added, err := mergeMegaBYVideoModels(raw)
	if err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE node_configs SET extra_config=?, updated_at=datetime('now','localtime') WHERE node_type='video'`, merged); err != nil {
		return err
	}
	log.Info("megaby video models registered", "added", added)
	return nil
}

func mergeMegaBYVideoModels(raw string) (string, int, error) {
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
	for _, preset := range megaBYVideoModels {
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
