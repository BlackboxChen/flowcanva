package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var geekNowImagePollInterval = 4 * time.Second
var geekNowImageMaxWait = 10 * time.Minute

func isGeekNowImageModel(model string) bool {
	switch model {
	case "gpt-image-2", "gpt-image-2-pro", "gpt-image-2-vip", "doubao-seedream-4-0-250828", "doubao-seedream-4-5-251128", "doubao-seedream-5-0-260128", "grok-4-2-image":
		return true
	default:
		return false
	}
}

func (h *Handler) generateGeekNowImage(c *gin.Context, req ImageGenRequest, baseURL, apiKey string) {
	size, err := geekNowImageSize(req.Model, req.ImageSize, req.AspectRatio)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	payload := map[string]any{
		"model":           req.Model,
		"prompt":          req.Prompt,
		"n":               max(req.N, 1),
		"size":            size,
		"response_format": "url",
	}
	references := req.Images
	if len(references) == 0 && req.Image != "" {
		references = []string{req.Image}
	}
	if len(references) > 0 {
		req.Size = size
		h.generateGeekNowImageEdit(c, req, references, strings.TrimRight(baseURL, "/"), apiKey)
		return
	}
	h.generateGeekNowImageAsync(c, strings.TrimRight(baseURL, "/"), apiKey, payload)
}

func (h *Handler) generateGeekNowImageEdit(c *gin.Context, req ImageGenRequest, references []string, baseURL, apiKey string) {
	body, contentType, err := writeImageEditMultipart(c.Request.Context(), req, references)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	defer func() {
		body.Close()
		_ = os.Remove(body.Name())
	}()

	httpReq, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, baseURL+"/images/edits", body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if info, statErr := body.Stat(); statErr == nil {
		httpReq.ContentLength = info.Size()
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", contentType)
	response, err := imageGenerationClient.Do(httpReq)
	if err != nil {
		h.log.Error("geeknow image edit failed", "error", err, "references", len(references))
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(response.Body)
	if response.StatusCode >= 400 {
		h.log.Error("geeknow image edit api error", "status", response.StatusCode, "references", len(references))
	}
	c.Data(response.StatusCode, "application/json", responseBody)
}

func (h *Handler) generateGeekNowImageAsync(c *gin.Context, baseURL, apiKey string, payload map[string]any) {
	task, err := requestGeekNowImageTask(c, baseURL, apiKey, payload)
	if err != nil {
		h.log.Error("geeknow image task create failed", "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	h.log.Info("geeknow image task created", "task_id", task)

	result, err := pollGeekNowImageTask(c, baseURL, apiKey, task)
	if err != nil {
		h.log.Error("geeknow image task failed", "task_id", task, "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "task_id": task})
		return
	}
	h.log.Info("geeknow image task completed", "task_id", task, "images", len(result.Data))
	c.JSON(http.StatusOK, result)
}

type geekNowImageResult struct {
	Created int64 `json:"created,omitempty"`
	Data    []struct {
		URL     string `json:"url,omitempty"`
		B64JSON string `json:"b64_json,omitempty"`
	} `json:"data"`
}

func requestGeekNowImageTask(c *gin.Context, baseURL, apiKey string, payload map[string]any) (string, error) {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, baseURL+"/images/generations/async", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := imageGenerationClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := decodeGeekNowImageResponse(resp)
	if err != nil {
		return "", err
	}
	for _, key := range []string{"id", "task_id", "taskId"} {
		if value, _ := data[key].(string); value != "" {
			return value, nil
		}
	}
	return "", fmt.Errorf("异步生图未返回 task_id")
}

func pollGeekNowImageTask(c *gin.Context, baseURL, apiKey, taskID string) (geekNowImageResult, error) {
	deadline := time.NewTimer(geekNowImageMaxWait)
	defer deadline.Stop()
	ticker := time.NewTicker(geekNowImagePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return geekNowImageResult{}, c.Request.Context().Err()
		case <-deadline.C:
			return geekNowImageResult{}, fmt.Errorf("异步生图任务超时")
		case <-ticker.C:
			req, _ := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, baseURL+"/images/generations/async/"+taskID, nil)
			req.Header.Set("Authorization", "Bearer "+apiKey)
			resp, err := pollClient.Do(req)
			if err != nil {
				continue
			}
			data, decodeErr := decodeGeekNowImageResponse(resp)
			resp.Body.Close()
			if decodeErr != nil {
				return geekNowImageResult{}, decodeErr
			}
			status, _ := data["status"].(string)
			status = strings.ToLower(status)
			if status == "failed" || status == "cancelled" || status == "canceled" {
				return geekNowImageResult{}, fmt.Errorf("异步生图失败: %s", geekNowImageError(data))
			}
			if raw, ok := data["data"]; ok {
				encoded, _ := json.Marshal(map[string]any{"created": time.Now().Unix(), "data": raw})
				var result geekNowImageResult
				if json.Unmarshal(encoded, &result) == nil && len(result.Data) > 0 {
					return result, nil
				}
			}
		}
	}
}

func decodeGeekNowImageResponse(resp *http.Response) (map[string]any, error) {
	body, _ := io.ReadAll(resp.Body)
	data := map[string]any{}
	_ = json.Unmarshal(body, &data)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GeekNow 返回 %d: %s", resp.StatusCode, geekNowImageError(data))
	}
	return data, nil
}

func geekNowImageError(data map[string]any) string {
	if errData, ok := data["error"].(map[string]any); ok {
		if message, _ := errData["message"].(string); message != "" {
			return message
		}
	}
	if message, _ := data["error"].(string); message != "" {
		return message
	}
	return "未知错误"
}

func geekNowImageSize(model, resolution, ratio string) (string, error) {
	ratio = strings.TrimSpace(ratio)
	if ratio == "" || ratio == "自适应" {
		ratio = "1:1"
	}
	resolution = strings.ToUpper(strings.TrimSpace(resolution))
	if resolution == "" {
		resolution = "1K"
	}

	standard := map[string]string{
		"1:1": "1024x1024", "4:3": "1536x1152", "3:2": "1536x1024",
		"2:3": "1024x1536", "16:9": "1920x1080", "9:16": "1080x1920",
	}
	pro2K := map[string]string{
		"1:1": "2048x2048", "4:3": "2048x1536", "3:2": "2560x1712",
		"2:3": "1712x2560", "16:9": "2048x1152", "9:16": "1152x2048",
	}
	pro4K := map[string]string{
		"1:1": "2880x2880", "4:3": "3840x2880", "3:2": "3840x2560",
		"2:3": "2560x3840", "16:9": "3840x2160", "9:16": "2160x3840",
	}
	vip := map[string]map[string]string{
		"1K": {"1:1": "1024x1024", "4:3": "1024x768", "3:4": "768x1024", "3:2": "1008x672", "2:3": "672x1008", "16:9": "1280x720", "9:16": "720x1280", "21:9": "1344x576"},
		"2K": {"1:1": "2048x2048", "4:3": "2304x1728", "3:4": "1728x2304", "3:2": "2496x1664", "2:3": "1664x2496", "16:9": "2560x1440", "9:16": "1440x2560", "21:9": "3024x1296"},
		"4K": {"1:1": "2880x2880", "4:3": "3264x2448", "3:4": "2448x3264", "3:2": "3504x2336", "2:3": "2336x3504", "16:9": "3840x2160", "9:16": "2160x3840", "21:9": "3808x1632"},
	}

	var sizes map[string]string
	switch model {
	case "gpt-image-2":
		sizes = standard
	case "gpt-image-2-pro":
		if resolution == "4K" {
			sizes = pro4K
		} else {
			sizes = pro2K
		}
	case "gpt-image-2-vip":
		sizes = vip[resolution]
	default:
		sizes = map[string]string{
			"1:1": "2048x2048", "4:3": "2560x1920", "3:4": "1920x2560", "3:2": "2560x1707",
			"2:3": "1707x2560", "16:9": "2560x1440", "9:16": "1440x2560", "21:9": "3024x1296",
		}
	}
	if size := sizes[ratio]; size != "" {
		return size, nil
	}
	return "", fmt.Errorf("模型 %s 不支持 %s / %s", model, resolution, ratio)
}
