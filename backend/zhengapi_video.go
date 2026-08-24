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

var zhengVideoPollInterval = 6 * time.Second
var zhengVideoMaxWait = 10 * time.Minute

func isZhengAPISDModel(model string) bool {
	return strings.HasPrefix(model, "sd-2.0-15s-") || strings.HasPrefix(model, "sd-2.0fast-15s-")
}

func (h *Handler) generateZhengAPIVideo(c *gin.Context, req VideoGenRequest, baseURL, apiKey string) {
	if !isZhengAPISDModel(req.Model) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的 ZhengAPI SD-2.0 模型"})
		return
	}
	payload, err := buildZhengVideoPayload(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	baseURL = strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/v1")
	h.log.Info("zhengapi video task creating", "model", req.Model, "image_refs", len(req.ImageURLs), "video_refs", len(req.VideoURLs), "audio_refs", len(req.AudioURLs))
	taskID, err := createZhengVideoTask(c, baseURL, apiKey, payload)
	if err != nil {
		h.log.Error("zhengapi video task create failed", "model", req.Model, "error", err)
		c.JSON(upstreamErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	h.log.Info("zhengapi video task created", "model", req.Model, "task_id", taskID)

	videoURL, err := pollZhengVideoTask(c, baseURL, apiKey, taskID)
	if err != nil {
		h.log.Error("zhengapi video task failed", "task_id", taskID, "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "task_id": taskID})
		return
	}
	videoURL, err = h.cacheZhengVideo(c, taskID, videoURL)
	if err != nil {
		h.log.Error("zhengapi video local save failed", "task_id", taskID, "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "视频生成完成，但保存到本地失败", "task_id": taskID})
		return
	}
	h.log.Info("zhengapi video task completed", "task_id", taskID, "cached", true)
	c.JSON(http.StatusOK, gin.H{
		"videos": []string{videoURL},
		"items":  []map[string]any{{"index": 0, "task_id": taskID, "video_url": videoURL}},
	})
}

func buildZhengVideoPayload(req VideoGenRequest) (map[string]any, error) {
	imageURLs := append([]string{}, req.ImageURLs...)
	if req.ImageURL != "" {
		imageURLs = append([]string{req.ImageURL}, imageURLs...)
	}
	if len(imageURLs) > 9 || len(req.VideoURLs) > 3 || len(req.AudioURLs) > 3 {
		return nil, fmt.Errorf("参考素材超限：图片最多 9 个，视频最多 3 个，音频最多 3 个")
	}
	media := make([]map[string]string, 0, len(imageURLs)+len(req.VideoURLs)+len(req.AudioURLs))
	for _, url := range imageURLs {
		media = append(media, map[string]string{"type": "reference_image", "url": url})
	}
	for _, url := range req.VideoURLs {
		media = append(media, map[string]string{"type": "reference_video", "url": url})
	}
	for _, url := range req.AudioURLs {
		media = append(media, map[string]string{"type": "reference_audio", "url": url})
	}
	input := map[string]any{"prompt": req.Prompt}
	if len(media) > 0 {
		input["media"] = media
	}
	return map[string]any{
		"model": req.Model, "prompt": req.Prompt, "input": input,
		"seconds": "15", "size": zhengVideoSize(req.Model),
	}, nil
}

func zhengVideoSize(model string) string {
	portrait := strings.Contains(model, "-9x16-")
	resolution := "480p"
	for _, value := range []string{"1080p", "720p", "480p"} {
		if strings.HasSuffix(model, "-"+value) {
			resolution = value
			break
		}
	}
	landscape := map[string]string{"480p": "854x480", "720p": "1280x720", "1080p": "1920x1080"}[resolution]
	if !portrait {
		return landscape
	}
	parts := strings.Split(landscape, "x")
	return parts[1] + "x" + parts[0]
}

func createZhengVideoTask(c *gin.Context, baseURL, apiKey string, payload map[string]any) (string, error) {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, baseURL+"/v1/videos", bytes.NewReader(body))
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
	data, err := decodeZhengVideoResponse(resp)
	if err != nil {
		return "", err
	}
	for _, key := range []string{"id", "task_id"} {
		if value, _ := data[key].(string); value != "" {
			return value, nil
		}
	}
	return "", fmt.Errorf("创建视频任务后未返回 task_id")
}

func pollZhengVideoTask(c *gin.Context, baseURL, apiKey, taskID string) (string, error) {
	deadline := time.NewTimer(zhengVideoMaxWait)
	defer deadline.Stop()
	ticker := time.NewTicker(zhengVideoPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return "", c.Request.Context().Err()
		case <-deadline.C:
			return "", fmt.Errorf("视频任务超时")
		case <-ticker.C:
			req, _ := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, baseURL+"/v1/videos/"+taskID, nil)
			req.Header.Set("Authorization", "Bearer "+apiKey)
			resp, err := pollClient.Do(req)
			if err != nil {
				continue
			}
			data, decodeErr := decodeZhengVideoResponse(resp)
			resp.Body.Close()
			if decodeErr != nil {
				return "", decodeErr
			}
			status, _ := data["status"].(string)
			switch strings.ToLower(status) {
			case "completed":
				if url := findVideoURL(data); url != "" {
					return url, nil
				}
				return "", fmt.Errorf("视频任务完成但未返回下载地址")
			case "failed":
				return "", fmt.Errorf("视频任务失败: %s", zhengVideoFailure(data))
			}
		}
	}
}

func (h *Handler) cacheZhengVideo(c *gin.Context, taskID, videoURL string) (string, error) {
	localURL, err := persistRemoteVideo(c.Request.Context(), videoURL)
	if err != nil {
		return "", err
	}
	h.log.Info("zhengapi video cached", "task_id", taskID, "url", localURL)
	return localURL, nil
}

func decodeZhengVideoResponse(resp *http.Response) (map[string]any, error) {
	body, _ := io.ReadAll(resp.Body)
	data := map[string]any{}
	_ = json.Unmarshal(body, &data)
	if resp.StatusCode >= 400 {
		message := zhengVideoFailure(data)
		return nil, fmt.Errorf("上游返回 %d: %s", resp.StatusCode, message)
	}
	return data, nil
}

func zhengVideoFailure(data map[string]any) string {
	if reason, _ := data["fail_reason"].(string); reason != "" {
		return reason
	}
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

func upstreamErrorStatus(err error) int {
	message := err.Error()
	for _, status := range []int{400, 401, 402, 429} {
		if strings.Contains(message, fmt.Sprintf("上游返回 %d", status)) {
			return status
		}
	}
	return http.StatusBadGateway
}
