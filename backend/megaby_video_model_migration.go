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

func init() {
	// MegaBY 将同一组视频能力暴露为多个清晰度/供应商模型；统一加前缀避免与 ZhengAPI 重名。
	ids := []string{
		"videos-standard-480p", "videos-standard-720p", "videos-standard-1080p", "videos-standard-4k",
		"videos-fast-480p", "videos-fast-720p", "videos-mini-480p", "videos-mini-720p",
		"seedance-2-5", "seedance-2-5-480p", "seedance-2-5-720p", "seedance-2.0-min",
		"sd-2.0-480p", "sd-2.0-720p", "sd-2.0-1080p", "sd-2.0-4k", "sd2.0-4k",
		"sd-mini-480p", "sd-mini-720p", "sd-mini", "sd-fast-480p", "sd-fast-720p",
		"minimax-h3", "minimax-h3-f", "minimax_h3_image_audio_to_video_v2", "minimax_h3_image_audio_to_video_v2_15s",
		"sd2-pro", "sd2-pro-933-480", "sd2-mini", "sd2-pro-max", "sd2-pro-max-480", "sd2-pro-max-fast",
		"alibaba/wan-3.0", "grok-imagine-video", "grok-imagine-video-1.5-fast", "grok-imagine-video-1.5-preview",
		"happyhorse-1.0", "kling-video-3.0", "sora-2", "sora-2-pro",
	}
	for _, id := range ids {
		megaBYVideoModels = append(megaBYVideoModels, map[string]any{"name": "megaby-" + id, "channel": "MegaBY", "base_url": "https://newapi.megabyai.cc", "protocol": "megaby_video", "parameters": map[string]any{}})
	}
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
