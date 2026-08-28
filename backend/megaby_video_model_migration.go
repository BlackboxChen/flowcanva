package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
)

var megaBYVideoModels = []map[string]any{
	{"name": "megaby-alibaba/wan-3.0", "channel": "MegaBY", "base_url": "https://newapi.megabyai.cc", "protocol": "megaby_video", "parameters": map[string]any{}},
}

func init() {
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
	// MegaBY 仅保留 Wan 3.0，清理此前已写入的其他 MegaBY 模型。
	filtered := make([]any, 0, len(models)+len(megaBYVideoModels))
	for _, item := range models {
		model, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := model["name"].(string)
		channel, _ := model["channel"].(string)
		if channel == "MegaBY" && name != "megaby-alibaba/wan-3.0" {
			continue
		}
		filtered = append(filtered, item)
	}
	models = filtered
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
