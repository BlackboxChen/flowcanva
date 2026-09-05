package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
)

var geekNowVideoModels = []map[string]any{}

func ensureGeekNowVideoModels(db *sql.DB, log *slog.Logger) error {
	var raw string
	if err := db.QueryRow(`SELECT extra_config FROM node_configs WHERE node_type='video'`).Scan(&raw); err != nil {
		return err
	}
	merged, added, err := mergeGeekNowVideoModels(raw)
	if err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE node_configs SET extra_config=?, updated_at=datetime('now','localtime') WHERE node_type='video'`, merged); err != nil {
		return err
	}
	log.Info("geeknow video models registered", "added", added)
	return nil
}

func mergeGeekNowVideoModels(raw string) (string, int, error) {
	extra := map[string]any{}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &extra); err != nil {
			return "", 0, fmt.Errorf("parse video extra_config: %w", err)
		}
	}
	models, _ := extra["models"].([]any)
	// GeekNow 的 wan3.0-video 已下线，从现有配置中移除。
	filtered := make([]any, 0, len(models))
	for _, item := range models {
		model, ok := item.(map[string]any)
		if ok && model["channel"] == "GeekNow" && model["name"] == "wan3.0-video" {
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
	for _, preset := range geekNowVideoModels {
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
