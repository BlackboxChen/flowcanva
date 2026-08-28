package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func (h *Handler) generateGeekNowVideo(c *gin.Context, req VideoGenRequest, baseURL, apiKey string) {
	if req.Model != "wan3.0-video" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的 GeekNow 视频模型"})
		return
	}
	payload, err := buildGeekNowVideoPayload(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.log.Info("geeknow video task creating", "model", req.Model)
	taskID, err := createGeekNowVideoTask(c, baseURL, apiKey, payload)
	if err != nil {
		c.JSON(upstreamErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	videoURL, err := pollGeekNowVideoTask(c, baseURL, apiKey, taskID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "task_id": taskID})
		return
	}
	localURL, err := persistRemoteVideo(c.Request.Context(), videoURL)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "视频生成完成，但保存到本地失败", "task_id": taskID})
		return
	}
	c.JSON(http.StatusOK, gin.H{"videos": []string{localURL}, "items": []map[string]any{{"index": 0, "task_id": taskID, "video_url": localURL}}})
}

func createGeekNowVideoTask(c *gin.Context, baseURL, apiKey string, payload map[string]any) (string, error) {
	body, _ := json.Marshal(payload)
	url := strings.TrimRight(baseURL, "/") + "/video/generations"
	if !strings.HasSuffix(strings.ToLower(strings.TrimRight(baseURL, "/")), "/v1") {
		url = strings.TrimRight(baseURL, "/") + "/v1/video/generations"
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := llmClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := decodeGeekNowResponse(resp)
	if err != nil {
		return "", err
	}
	for _, key := range []string{"id", "task_id", "video_id"} {
		if id, _ := data[key].(string); id != "" {
			return id, nil
		}
	}
	return "", fmt.Errorf("创建视频任务后未返回 task_id")
}

func pollGeekNowVideoTask(c *gin.Context, baseURL, apiKey, taskID string) (string, error) {
	base := strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(strings.ToLower(base), "/v1") {
		base += "/v1"
	}
	for deadline := time.Now().Add(10 * time.Minute); time.Now().Before(deadline); {
		select {
		case <-c.Request.Context().Done():
			return "", c.Request.Context().Err()
		case <-time.After(5 * time.Second):
		}
		req, _ := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, base+"/video/generations/"+taskID, nil)
		req.Header.Set("Authorization", "Bearer "+apiKey)
		resp, err := pollClient.Do(req)
		if err != nil {
			continue
		}
		data, decodeErr := decodeGeekNowResponse(resp)
		resp.Body.Close()
		if decodeErr != nil {
			return "", decodeErr
		}
		status := strings.ToLower(extractStatus(data))
		if url := findVideoURL(data); url != "" && (status == "" || status == "completed" || status == "succeeded" || status == "success" || status == "done") {
			return url, nil
		}
		if status == "failed" || status == "failure" || status == "error" || status == "cancelled" {
			return "", fmt.Errorf("视频任务失败: %s", status)
		}
	}
	return "", fmt.Errorf("视频任务超时")
}

func decodeGeekNowResponse(resp *http.Response) (map[string]any, error) {
	body, _ := io.ReadAll(resp.Body)
	data := map[string]any{}
	_ = json.Unmarshal(body, &data)
	if resp.StatusCode >= 400 {
		message := zhengVideoFailure(data)
		if message == "未知错误" {
			message = strings.TrimSpace(string(body))
		}
		return nil, fmt.Errorf("上游返回 %d: %s", resp.StatusCode, message)
	}
	return data, nil
}

func buildGeekNowVideoPayload(req VideoGenRequest) (map[string]any, error) {
	duration := 5
	if req.Seconds != "" {
		var err error
		if _, err = fmt.Sscanf(req.Seconds, "%d", &duration); err != nil || duration < 2 || duration > 30 {
			return nil, fmt.Errorf("视频时长必须为 2-30 秒")
		}
	}
	payload := map[string]any{"model": "wan3.0-video", "prompt": req.Prompt, "duration": duration}
	if req.Resolution != "" {
		payload["resolution"] = strings.ToUpper(req.Resolution)
	}
	if req.Ratio != "" {
		payload["aspect_ratio"] = req.Ratio
	}
	images := append([]string{}, req.ImageURLs...)
	if req.ImageURL != "" {
		images = append([]string{req.ImageURL}, images...)
	}
	media := make([]map[string]string, 0, len(images)+len(req.VideoURLs)+len(req.AudioURLs))
	for _, url := range images {
		media = append(media, map[string]string{"type": "image", "url": url})
	}
	for _, url := range req.VideoURLs {
		media = append(media, map[string]string{"type": "video", "url": url})
	}
	for _, url := range req.AudioURLs {
		media = append(media, map[string]string{"type": "audio", "url": url})
	}
	if len(media) > 0 {
		payload["media"] = media
	}
	return payload, nil
}
