package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func fetchWorkflowHistory(ctx context.Context, j workflowJob) ([]map[string]string, bool, error) {
	request, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(j.Server, "/")+"/history/"+url.PathEscape(j.Prompt), nil)
	if err != nil {
		return nil, false, err
	}
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		return nil, false, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, false, fmt.Errorf("历史接口 HTTP %d", response.StatusCode)
	}
	var history map[string]struct {
		Outputs map[string]map[string]json.RawMessage `json:"outputs"`
		Status  map[string]any                        `json:"status"`
	}
	if err = json.NewDecoder(response.Body).Decode(&history); err != nil {
		return nil, false, err
	}
	entry, found := history[j.Prompt]
	if !found {
		return nil, false, nil
	}
	if entry.Status["status_str"] == "error" {
		return nil, true, fmt.Errorf("%s", comfyExecutionError(entry.Status))
	}
	if completed, ok := entry.Status["completed"].(bool); ok && !completed {
		return nil, false, nil
	}
	keys := []string{}
	for key := range entry.Outputs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := []map[string]string{}
	for _, nodeID := range keys {
		node := entry.Outputs[nodeID]
		for _, bucket := range []string{"images", "gifs", "videos", "audio"} {
			var files []map[string]any
			if json.Unmarshal(node[bucket], &files) != nil {
				continue
			}
			for _, file := range files {
				filename, _ := file["filename"].(string)
				if filename == "" {
					continue
				}
				sub, _ := file["subfolder"].(string)
				fileType, _ := file["type"].(string)
				if fileType == "" {
					fileType = "output"
				}
				format, _ := file["format"].(string)
				kind := "image"
				switch strings.ToLower(filepath.Ext(filename)) {
				case ".mp4", ".webm", ".mov", ".mkv":
					kind = "video"
				case ".wav", ".mp3", ".flac", ".ogg", ".m4a":
					kind = "audio"
				}
				if bucket == "audio" {
					kind = "audio"
				}
				query := url.Values{"filename": {filename}, "subfolder": {sub}, "type": {fileType}}
				if format != "" {
					query.Set("format", format)
				}
				result = append(result, map[string]string{"url": strings.TrimRight(j.Server, "/") + "/view?" + query.Encode(), "filename": filename, "kind": kind, "node_id": nodeID})
			}
		}
		var texts []string
		if json.Unmarshal(node["text"], &texts) == nil {
			for _, text := range texts {
				result = append(result, map[string]string{"kind": "text", "value": text, "node_id": nodeID})
			}
		}
	}
	if len(result) == 0 {
		return nil, true, fmt.Errorf("工作流已结束但没有可保存的媒体或文本输出")
	}
	return result, true, nil
}

// Ambiguous submission recovery NEVER resubmits a generation request.
func findSubmittedWorkflow(ctx context.Context, j workflowJob) (string, error) {
	for _, endpoint := range []string{"/queue", "/history?max_items=200"} {
		req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(j.Server, "/")+endpoint, nil)
		if err != nil {
			return "", err
		}
		resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
		if err != nil {
			return "", err
		}
		var data map[string]json.RawMessage
		err = json.NewDecoder(resp.Body).Decode(&data)
		resp.Body.Close()
		if err != nil {
			return "", err
		}
		tuples := [][]json.RawMessage{}
		if endpoint == "/queue" {
			for _, key := range []string{"queue_running", "queue_pending"} {
				var list [][]json.RawMessage
				_ = json.Unmarshal(data[key], &list)
				tuples = append(tuples, list...)
			}
		} else {
			for _, entry := range data {
				var item struct {
					Prompt []json.RawMessage `json:"prompt"`
				}
				if json.Unmarshal(entry, &item) == nil {
					tuples = append(tuples, item.Prompt)
				}
			}
		}
		for _, tuple := range tuples {
			if len(tuple) < 4 {
				continue
			}
			var meta map[string]any
			_ = json.Unmarshal(tuple[3], &meta)
			if meta["flowcanva_job_id"] == j.ID || meta["client_id"] == j.ID {
				var prompt string
				_ = json.Unmarshal(tuple[1], &prompt)
				if prompt != "" {
					return prompt, nil
				}
			}
		}
	}
	return "", nil
}
