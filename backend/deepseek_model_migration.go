package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// DeepSeek 官方渠道接入。
// V4.1 Flash 在 API 侧的模型名是 deepseek-flash（旧名 deepseek-v4-flash 会被官方路由到它）。
// 密钥只从环境变量读取，不写入任何受版本控制的文件。
const (
	deepSeekChannel     = "DeepSeek"
	deepSeekModelName   = "deepseek-flash"
	deepSeekDefaultBase = "https://api.deepseek.com/v1"
)

// deepSeekTarget 描述一个挂载 DeepSeek 渠道的节点。
type deepSeekTarget struct {
	NodeType string
	// Parameters 写入候选模型条目的默认超参。
	Parameters map[string]any
	// AdoptDefault 为 true 时，仅在首次注入（列表里还没有该模型）才把节点主模型切过去，
	// 之后保留用户在管理台的选择，避免每次启动覆盖人工配置。
	AdoptDefault bool
}

// deepSeekTargets 是使用该渠道的文本类节点。
// 剧本分析由 shotlist（分镜台）承担；text / agent 属同类文本工作，一并提供候选。
var deepSeekTargets = []deepSeekTarget{
	{NodeType: "shotlist", Parameters: map[string]any{"temperature": 0.4, "max_tokens": 32768}, AdoptDefault: true},
	{NodeType: "text", Parameters: map[string]any{"temperature": 0.7, "max_tokens": 32768, "supports_vision": true}},
	{NodeType: "agent", Parameters: map[string]any{"temperature": 0.7, "max_tokens": 32768, "supports_vision": true}},
}

// ensureDeepSeekModel 把 DeepSeek 渠道注入目标节点的候选模型列表。
// 未配置密钥时静默跳过，不影响其他渠道。
func ensureDeepSeekModel(db *sql.DB, log *slog.Logger) error {
	apiKey := strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
	if apiKey == "" {
		log.Info("deepseek channel skipped", "reason", "DEEPSEEK_API_KEY not set")
		return nil
	}
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("DEEPSEEK_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = deepSeekDefaultBase
	}

	for _, target := range deepSeekTargets {
		if err := upsertDeepSeekModel(db, target, baseURL, apiKey); err != nil {
			return fmt.Errorf("deepseek channel for %s: %w", target.NodeType, err)
		}
	}
	log.Info("deepseek channel registered", "model", deepSeekModelName, "nodes", len(deepSeekTargets))
	return nil
}

func upsertDeepSeekModel(db *sql.DB, target deepSeekTarget, baseURL, apiKey string) error {
	var raw, currentModel string
	err := db.QueryRow(
		`SELECT extra_config, model_name FROM node_configs WHERE node_type=?`,
		target.NodeType,
	).Scan(&raw, &currentModel)
	if errors.Is(err, sql.ErrNoRows) {
		// 该节点类型尚未初始化，跳过即可。
		return nil
	}
	if err != nil {
		return err
	}

	extra := map[string]any{}
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &extra); err != nil {
			return fmt.Errorf("parse extra_config: %w", err)
		}
	}
	models, _ := extra["models"].([]any)

	entry := map[string]any{
		"name":       deepSeekModelName,
		"channel":    deepSeekChannel,
		"base_url":   baseURL,
		"api_key":    apiKey,
		"parameters": target.Parameters,
	}

	// 同名条目原地替换，保持列表原有顺序，避免候选模型顺序抖动。
	replaced := false
	for i, item := range models {
		model, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if name, _ := model["name"].(string); name == deepSeekModelName {
			models[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		models = append(models, entry)
	}
	extra["models"] = models

	merged, err := json.Marshal(extra)
	if err != nil {
		return err
	}

	if target.AdoptDefault && !replaced {
		_, err = db.Exec(
			`UPDATE node_configs SET extra_config=?, model_name=?, api_channel=?, base_url=?, api_key=?,
			 updated_at=datetime('now','localtime') WHERE node_type=?`,
			string(merged), deepSeekModelName, deepSeekChannel, baseURL, apiKey, target.NodeType,
		)
		return err
	}

	_, err = db.Exec(
		`UPDATE node_configs SET extra_config=?, updated_at=datetime('now','localtime') WHERE node_type=?`,
		string(merged), target.NodeType,
	)
	return err
}
