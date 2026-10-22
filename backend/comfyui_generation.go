package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type comfyGenerationOutput struct {
	URL  string
	Kind string
}

func (h *Handler) generateWithComfyUI(c *gin.Context, serverURL, workflowTemplate, prompt string, replacements map[string]any) ([]comfyGenerationOutput, error) {
	serverURL = strings.TrimRight(strings.TrimSpace(serverURL), "/")
	if serverURL == "" {
		return nil, fmt.Errorf("请在节点模型配置中填写 ComfyUI 服务地址")
	}
	if strings.TrimSpace(workflowTemplate) == "" {
		return nil, fmt.Errorf("请在节点模型配置中填写 ComfyUI API 工作流 JSON")
	}

	values := map[string]any{"prompt": prompt}
	for key, value := range replacements {
		values[key] = value
	}
	workflow, err := replaceComfyWorkflowValues(workflowTemplate, values)
	if err != nil {
		return nil, fmt.Errorf("ComfyUI 工作流 JSON 无效: %w", err)
	}
	body, _ := json.Marshal(map[string]json.RawMessage{"prompt": workflow})
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Minute)
	defer cancel()
	client := &http.Client{Timeout: 20 * time.Second}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/prompt", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("无法连接 ComfyUI: %w", err)
	}
	var submitted struct {
		PromptID string          `json:"prompt_id"`
		Error    json.RawMessage `json:"error"`
	}
	decodeErr := json.NewDecoder(resp.Body).Decode(&submitted)
	resp.Body.Close()
	if decodeErr != nil {
		return nil, fmt.Errorf("无法解析 ComfyUI 提交响应: %w", decodeErr)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ComfyUI 提交失败 (%d): %s", resp.StatusCode, string(submitted.Error))
	}
	if submitted.PromptID == "" {
		return nil, fmt.Errorf("ComfyUI 未返回 prompt_id: %s", string(submitted.Error))
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("等待 ComfyUI 任务完成超时或已取消")
		case <-ticker.C:
			outputs, done, pollErr := pollComfyUIHistory(ctx, client, serverURL, submitted.PromptID)
			if pollErr != nil {
				return nil, pollErr
			}
			if done {
				if len(outputs) == 0 {
					return nil, fmt.Errorf("ComfyUI 已完成，但工作流没有返回图片或视频输出")
				}
				return outputs, nil
			}
		}
	}
}

func replaceComfyWorkflowValues(template string, values map[string]any) (json.RawMessage, error) {
	var workflow any
	if err := json.Unmarshal([]byte(template), &workflow); err != nil {
		return nil, err
	}
	workflow = replaceComfyValue(workflow, values)
	return json.Marshal(workflow)
}

func replaceComfyValue(value any, values map[string]any) any {
	switch current := value.(type) {
	case string:
		if replacement, ok := values[strings.Trim(current, "{}")]; ok && strings.HasPrefix(current, "{{") && strings.HasSuffix(current, "}}") {
			return replacement
		}
		for key, replacement := range values {
			current = strings.ReplaceAll(current, "{{"+key+"}}", fmt.Sprint(replacement))
		}
		return current
	case []any:
		for i := range current {
			current[i] = replaceComfyValue(current[i], values)
		}
	case map[string]any:
		for key := range current {
			current[key] = replaceComfyValue(current[key], values)
		}
	}
	return value
}

func pollComfyUIHistory(ctx context.Context, client *http.Client, serverURL, promptID string) ([]comfyGenerationOutput, bool, error) {
	endpoint := serverURL + "/history/" + url.PathEscape(promptID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, false, err
	}
	resp, err := client.Do(request)
	if err != nil {
		return nil, false, fmt.Errorf("读取 ComfyUI 历史失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, false, fmt.Errorf("读取 ComfyUI 历史失败 (%d)", resp.StatusCode)
	}
	var history map[string]struct {
		Outputs map[string]struct {
			Images []struct {
				Filename  string `json:"filename"`
				Subfolder string `json:"subfolder"`
				Type      string `json:"type"`
			} `json:"images"`
			Gifs []struct {
				Filename  string `json:"filename"`
				Subfolder string `json:"subfolder"`
				Format    string `json:"format"`
			} `json:"gifs"`
		} `json:"outputs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&history); err != nil {
		return nil, false, fmt.Errorf("无法解析 ComfyUI 历史数据: %w", err)
	}
	entry, done := history[promptID]
	if !done {
		return nil, false, nil
	}
	results := []comfyGenerationOutput{}
	for _, node := range entry.Outputs {
		for _, image := range node.Images {
			query := url.Values{"filename": {image.Filename}, "subfolder": {image.Subfolder}, "type": {image.Type}}
			kind := "image"
			lowerName := strings.ToLower(image.Filename)
			if strings.HasSuffix(lowerName, ".mp4") || strings.HasSuffix(lowerName, ".webm") || strings.HasSuffix(lowerName, ".mov") || strings.HasSuffix(lowerName, ".gif") {
				kind = "video"
			}
			results = append(results, comfyGenerationOutput{URL: serverURL + "/view?" + query.Encode(), Kind: kind})
		}
		for _, video := range node.Gifs {
			query := url.Values{"filename": {video.Filename}, "subfolder": {video.Subfolder}, "type": {"output"}, "format": {video.Format}}
			results = append(results, comfyGenerationOutput{URL: serverURL + "/view?" + query.Encode(), Kind: "video"})
		}
	}
	return results, true, nil
}

func (h *Handler) generateComfyUIImage(c *gin.Context, req ImageGenRequest) {
	width, height := comfyImageDimensions(req.Size)
	imageRefs := append([]string{}, req.Images...)
	if len(imageRefs) == 0 && req.Image != "" {
		imageRefs = append(imageRefs, req.Image)
	}
	uploadedRefs, err := uploadComfyImageReferences(c, req.BaseURL, imageRefs)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	outputs, err := h.generateWithComfyUI(c, req.BaseURL, req.WorkflowJSON, req.Prompt, map[string]any{
		"width":  width,
		"height": height,
		"size":   req.Size,
		"count":  req.N,
		"image":  firstComfyInput(uploadedRefs),
		"images": uploadedRefs,
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	items := make([]map[string]string, 0, len(outputs))
	for _, output := range outputs {
		if output.Kind != "image" {
			continue
		}
		items = append(items, map[string]string{"url": "/api/comfyui/proxy-view?url=" + url.QueryEscape(output.URL)})
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func comfyImageDimensions(size string) (int, int) {
	var width, height int
	if _, err := fmt.Sscanf(size, "%dx%d", &width, &height); err != nil || width <= 0 || height <= 0 {
		return 1024, 1024
	}
	return width, height
}

func (h *Handler) generateComfyUIVideo(c *gin.Context, req VideoGenRequest) {
	imageRefs := append([]string{}, req.ImageURLs...)
	if req.ImageURL != "" {
		imageRefs = append([]string{req.ImageURL}, imageRefs...)
	}
	uploadedRefs, err := uploadComfyImageReferences(c, req.BaseURL, imageRefs)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	outputs, err := h.generateWithComfyUI(c, req.BaseURL, req.WorkflowJSON, req.Prompt, map[string]any{
		"seconds":    req.Seconds,
		"ratio":      req.Ratio,
		"resolution": req.Resolution,
		"count":      req.Count,
		"image":      firstComfyInput(uploadedRefs),
		"images":     uploadedRefs,
		"videos":     req.VideoURLs,
		"audios":     req.AudioURLs,
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	videos := []string{}
	for _, output := range outputs {
		if output.Kind != "video" {
			continue
		}
		localURL, saveErr := persistRemoteVideo(c.Request.Context(), output.URL)
		if saveErr != nil {
			h.log.Error("comfyui video cache failed", "error", saveErr)
			c.JSON(http.StatusBadGateway, gin.H{"error": "ComfyUI 视频已生成，但保存到本地失败"})
			return
		}
		videos = append(videos, localURL)
	}
	if len(videos) == 0 {
		c.JSON(http.StatusBadGateway, gin.H{"error": "ComfyUI 工作流未返回视频输出"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"videos": videos})
}

func firstComfyInput(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
