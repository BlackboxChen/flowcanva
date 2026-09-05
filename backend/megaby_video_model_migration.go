package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
)

var megaBYVideoModels = []map[string]any{
	{"name": "megaby-alibaba/wan-3.0", "channel": "MegaBY", "base_url": "https://newapi.megabyai.cc", "protocol": "megaby_video", "parameters": map[string]any{}},
	{"name": "seedance-2.0", "channel": "MegaBY", "base_url": "https://newapi.megabyai.cc", "protocol": "megaby_video", "parameters": map[string]any{"note": "0.16"}},
	{"name": "seedance-2.5", "channel": "MegaBY", "base_url": "https://newapi.megabyai.cc", "protocol": "megaby_video", "parameters": map[string]any{"note": "0.26"}},
	{"name": "seedance2_5", "channel": "MegaBY", "base_url": "https://newapi.megabyai.cc", "protocol": "megaby_video", "parameters": map[string]any{"note": "0.24"}},
	{"name": "videos-standard", "channel": "MegaBY", "base_url": "https://newapi.megabyai.cc", "protocol": "megaby_video", "parameters": map[string]any{"note": "0.28"}},
	{"name": "seedance-2-5", "channel": "MegaBY", "base_url": "https://newapi.megabyai.cc", "protocol": "megaby_video", "parameters": map[string]any{"note": "0.35"}},
	{"name": "sd-2.0-480p", "channel": "MegaBY", "base_url": "https://newapi.megabyai.cc", "protocol": "megaby_video", "parameters": map[string]any{"note": "0.5"}},
	{"name": "minimax-h3-f", "channel": "MegaBY", "base_url": "https://newapi.megabyai.cc", "protocol": "megaby_video", "parameters": map[string]any{"note": "0.03"}},
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
	merged, err = orderVideoModelsByChannel(merged)
	if err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE node_configs SET extra_config=?, updated_at=datetime('now','localtime') WHERE node_type='video'`, merged); err != nil {
		return err
	}
	log.Info("megaby video models registered", "added", added)
	return nil
}

func orderVideoModelsByChannel(raw string) (string, error) {
	extra := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &extra); err != nil {
		return "", fmt.Errorf("parse ordered video extra_config: %w", err)
	}
	models, _ := extra["models"].([]any)
	priority := map[string]int{"ZhengAPI": 0, "MegaBY": 1, "GeekNow": 2}
	sort.SliceStable(models, func(i, j int) bool {
		left, _ := models[i].(map[string]any)
		right, _ := models[j].(map[string]any)
		leftChannel, _ := left["channel"].(string)
		rightChannel, _ := right["channel"].(string)
		lp, lok := priority[leftChannel]
		rp, rok := priority[rightChannel]
		if !lok {
			lp = 99
		}
		if !rok {
			rp = 99
		}
		return lp < rp
	})
	extra["models"] = models
	ordered, err := json.Marshal(extra)
	return string(ordered), err
}

func mergeMegaBYVideoModels(raw string) (string, int, error) {
	extra := map[string]any{}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &extra); err != nil {
			return "", 0, fmt.Errorf("parse video extra_config: %w", err)
		}
	}
	models, _ := extra["models"].([]any)
	// 清理此前已写入、但不在当前 MegaBY 预置列表中的模型。
	filtered := make([]any, 0, len(models)+len(megaBYVideoModels))
	for _, item := range models {
		model, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := model["name"].(string)
		channel, _ := model["channel"].(string)
		if channel == "MegaBY" && !isConfiguredMegaBYModel(name) {
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

func isConfiguredMegaBYModel(name string) bool {
	for _, model := range megaBYVideoModels {
		if model["name"] == name {
			return true
		}
	}
	return false
}
