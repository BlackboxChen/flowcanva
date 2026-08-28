package main

import (
	"fmt"
	"net/http"
	"strings"

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
	taskID, err := createZhengVideoTask(c, strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/v1"), apiKey, payload)
	if err != nil {
		c.JSON(upstreamErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	videoURL, err := pollZhengVideoTask(c, strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/v1"), apiKey, taskID)
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
