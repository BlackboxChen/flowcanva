package main

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strconv"
)

type workflowDefinition struct {
	Server    string `json:"serverUrl"`
	Graph     string `json:"workflowJson"`
	Approval  bool   `json:"approvalRequired"`
	SeedMode  string `json:"seedMode"`
	FixedSeed any    `json:"fixedSeed"`
	Job       string `json:"workflowJobID"`
	Variables []struct {
		Name   string `json:"name"`
		Value  any    `json:"value"`
		Type   string `json:"type"`
		Source string `json:"sourceNodeId"`
	} `json:"variables"`
}

func (h *Handler) batchRequest(t batchTask, raw string) (comfyExecuteRequest, error) {
	var data workflowDefinition
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return comfyExecuteRequest{}, err
	}
	req := comfyExecuteRequest{ServerURL: data.Server, CanvasID: t.Canvas, NodeID: t.Node, Variables: map[string]any{}}
	if data.Server == "" {
		return req, fmt.Errorf("生成服务地址为空")
	}
	var graph any
	if err := json.Unmarshal([]byte(data.Graph), &graph); err != nil {
		return req, err
	}
	if data.SeedMode == "random" || data.SeedMode == "fixed" {
		var seed int64
		if data.SeedMode == "random" {
			var buf [4]byte
			if _, err := rand.Read(buf[:]); err != nil {
				return req, err
			}
			seed = int64(binary.LittleEndian.Uint32(buf[:]) & 0x7fffffff)
		} else {
			var err error
			seed, err = strconv.ParseInt(fmt.Sprint(data.FixedSeed), 10, 64)
			if err != nil || seed < 0 || seed > 9007199254740991 {
				return req, fmt.Errorf("固定种子无效")
			}
		}
		req.Seed = &seed
		replaceBatchSeeds(graph, seed)
	}
	req.WorkflowJSON, _ = json.Marshal(graph)
	for _, v := range data.Variables {
		value := v.Value
		if v.Source != "" {
			var sourceRaw string
			if err := h.db.QueryRow(`SELECT content FROM nodes WHERE id=? AND canvas_id=?`, v.Source, t.Canvas).Scan(&sourceRaw); err != nil {
				return req, fmt.Errorf("上游节点不存在: %s", v.Source)
			}
			var source map[string]any
			_ = json.Unmarshal([]byte(sourceRaw), &source)
			value = source["url"]
			if value == nil {
				value = source["dataUrl"]
			}
			if outputs, ok := source["outputs"].([]any); ok {
				for _, item := range outputs {
					if out, ok := item.(map[string]any); ok && out["type"] == v.Type {
						value = out["value"]
						break
					}
				}
			}
			if value == nil || value == "" {
				return req, fmt.Errorf("上游 %s 尚无 %s 输出", v.Source, v.Type)
			}
		}
		req.Variables[v.Name] = value
		if v.Type == "image" {
			req.ImageVariables = append(req.ImageVariables, v.Name)
		}
	}
	return req, nil
}
func replaceBatchSeeds(value any, seed int64) {
	switch data := value.(type) {
	case map[string]any:
		for k, v := range data {
			if k == "seed" || k == "noise_seed" || k == "random_seed" {
				switch v.(type) {
				case string, float64:
					data[k] = seed
					continue
				}
			}
			replaceBatchSeeds(v, seed)
		}
	case []any:
		for _, v := range data {
			replaceBatchSeeds(v, seed)
		}
	}
}

// Reference values are refreshed from their bound upstream node when dispatched.
func batchEditableContent(raw string) string {
	var data map[string]any
	if json.Unmarshal([]byte(editableContent(raw)), &data) != nil {
		return raw
	}
	if variables, ok := data["variables"].([]any); ok {
		for _, v := range variables {
			if variable, ok := v.(map[string]any); ok && variable["sourceNodeId"] != nil && variable["sourceNodeId"] != "" {
				delete(variable, "value")
			}
		}
	}
	result, _ := json.Marshal(data)
	return string(result)
}
