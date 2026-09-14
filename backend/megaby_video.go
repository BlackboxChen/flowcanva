package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func isMegaBYVideoModel(model string) bool {
	return strings.HasPrefix(model, "megaby-") || isConfiguredMegaBYModel(model)
}

func megaBYModelName(model string) string { return strings.TrimPrefix(model, "megaby-") }

func (h *Handler) generateMegaBYVideo(c *gin.Context, req VideoGenRequest, baseURL, apiKey string) {
	if !isConfiguredMegaBYModel(req.Model) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的 MegaBY 视频模型"})
		return
	}
	payload, err := buildMegaBYVideoPayload(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.log.Info("megaby video task creating", "model", req.Model)
	taskID, err := createZhengVideoTask(c, strings.TrimRight(baseURL, "/"), apiKey, payload)
	if err != nil {
		c.JSON(upstreamErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	videoURL, err := pollZhengVideoTask(c, strings.TrimRight(baseURL, "/"), apiKey, taskID)
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

func buildMegaBYVideoPayload(req VideoGenRequest) (map[string]any, error) {
	images := append([]string{}, req.ImageURLs...)
	if req.ImageURL != "" {
		images = append([]string{req.ImageURL}, images...)
	}
	if len(images) > 9 || len(req.VideoURLs) > 3 || len(req.AudioURLs) > 3 {
		return nil, fmt.Errorf("参考素材超限：图片最多 9 个，视频最多 3 个，音频最多 3 个")
	}
	duration := 5
	if req.Seconds != "" {
		value, err := strconv.Atoi(req.Seconds)
		if err != nil || value < 2 || value > 30 {
			return nil, fmt.Errorf("视频时长必须为 2-30 秒")
		} else {
			duration = value
		}
	}
	payload := map[string]any{"model": megaBYModelName(req.Model), "prompt": req.Prompt, "duration": duration}
	if req.Ratio != "" {
		// MegaBY's /v1/videos contract uses aspect_ratio. Keep ratio as a
		// compatibility alias for older gateway deployments, but always send the
		// canonical field so 16:9 is not silently treated as the default 1:1.
		payload["aspect_ratio"] = req.Ratio
		payload["ratio"] = req.Ratio
	}
	if req.Resolution != "" {
		payload["resolution"] = req.Resolution
	}
	if len(images) > 0 {
		payload["referenceImages"] = images
	}
	if len(req.VideoURLs) > 0 {
		payload["referenceVideos"] = req.VideoURLs
	}
	if len(req.AudioURLs) > 0 {
		payload["referenceAudios"] = req.AudioURLs
	}
	return payload, nil
}

// MegaBY 与 ZhengAPI 使用相同的任务响应结构，复用轮询/错误解析逻辑。
