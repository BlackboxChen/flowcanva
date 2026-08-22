package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	imageGenerationClient = &http.Client{Timeout: 300 * time.Second}
	fireflySuffixPattern  = regexp.MustCompile(`-(1k|2k|4k)-(auto|[0-9]+x[0-9]+)$`)
	markdownImagePattern  = regexp.MustCompile(`!\[[^\]]*\]\((https?://[^\s)]+|/v1/[^\s)]+)\)`)
	absoluteImagePattern  = regexp.MustCompile(`https?://[^\s)"']+`)
	relativeImagePattern  = regexp.MustCompile(`/v1/[^\s)"']+`)
	dataImagePattern      = regexp.MustCompile(`data:image/[^;\s]+;base64,[A-Za-z0-9+/=]+`)
)

func isGrokImageModel(model string) bool {
	return model == "grok-imagine-1.0" || model == "grok-imagine-1.0-edit"
}

func isFireflyImageModel(model string) bool {
	return strings.HasPrefix(model, "firefly-nano-banana") || strings.HasPrefix(model, "firefly-gpt-image")
}

func (h *Handler) generateGrokImage(c *gin.Context, req ImageGenRequest, baseURL, apiKey string) {
	if req.Model == "grok-imagine-1.0-edit" && req.Image == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "grok-imagine-1.0-edit 需要参考图"})
		return
	}

	payload := map[string]any{"model": req.Model, "prompt": req.Prompt, "n": max(req.N, 1)}
	if req.Image != "" {
		payload["image"] = stripDataURLPrefix(req.Image)
	}
	h.proxyImageJSON(c, baseURL+"/images/generations", apiKey, payload)
}

func (h *Handler) generateFireflyImage(c *gin.Context, req ImageGenRequest, baseURL, apiKey string) {
	model, err := buildFireflyModelID(req.Model, req.ImageSize, req.AspectRatio)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	content := make([]map[string]any, 0, 2)
	if req.Image != "" {
		content = append(content, map[string]any{
			"type":      "image_url",
			"image_url": map[string]string{"url": req.Image},
		})
	}
	content = append(content, map[string]any{"type": "text", "text": req.Prompt})
	payload := map[string]any{
		"model":    model,
		"messages": []map[string]any{{"role": "user", "content": content}},
		"stream":   true,
	}

	body, _ := json.Marshal(payload)
	httpReq, err := http.NewRequest(http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := imageGenerationClient.Do(httpReq)
	if err != nil {
		h.log.Error("firefly image request failed", "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		h.log.Error("firefly image api error", "status", resp.StatusCode)
		c.Data(resp.StatusCode, "application/json", respBody)
		return
	}

	imageURL, err := parseFireflyImageResponse(respBody, baseURL)
	if err != nil {
		h.log.Error("firefly image response parse failed", "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if strings.HasPrefix(imageURL, "data:image/") {
		imageURL, err = persistEmbeddedImage(imageURL)
		if err != nil {
			h.log.Error("firefly image save failed", "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "生成图片保存失败"})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": []gin.H{{"url": imageURL}}})
}

func (h *Handler) proxyImageJSON(c *gin.Context, endpoint, apiKey string, payload map[string]any) {
	body, _ := json.Marshal(payload)
	httpReq, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := imageGenerationClient.Do(httpReq)
	if err != nil {
		h.log.Error("image request failed", "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		h.log.Error("image api error", "status", resp.StatusCode)
		c.Data(resp.StatusCode, "application/json", respBody)
		return
	}
	materialized, err := materializeImageResponse(respBody)
	if err != nil {
		h.log.Error("image response materialization failed", "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "生成图片保存失败"})
		return
	}
	c.Data(resp.StatusCode, "application/json", materialized)
}

func buildFireflyModelID(model, resolution, ratio string) (string, error) {
	if fireflySuffixPattern.MatchString(strings.ToLower(model)) {
		return model, nil
	}
	allowed := map[string]map[string]bool{
		"firefly-nano-banana":     stringSet("1x1", "16x9", "9x16", "4x3", "3x4", "21x9", "5x4", "4x5"),
		"firefly-nano-banana-pro": stringSet("1x1", "16x9", "9x16", "4x3", "3x4", "21x9", "5x4", "4x5"),
		"firefly-nano-banana2":    stringSet("1x1", "16x9", "9x16", "4x3", "3x4", "21x9", "3x2", "2x3", "5x4", "4x5", "8x1", "1x4", "1x8"),
		"firefly-gpt-image":       stringSet("auto", "1x1", "16x9", "9x16", "21x9", "4x3", "3x2", "2x3", "5x4", "4x5", "3x4"),
	}
	ratios, ok := allowed[model]
	if !ok {
		return "", fmt.Errorf("不支持的 Firefly 图片模型: %s", model)
	}

	resolution = strings.ToLower(strings.TrimSpace(resolution))
	if resolution == "" {
		resolution = "1k"
	}
	if resolution != "1k" && resolution != "2k" && resolution != "4k" {
		return "", fmt.Errorf("不支持的分辨率: %s", resolution)
	}
	ratio = strings.ToLower(strings.TrimSpace(ratio))
	if ratio == "" || ratio == "自适应" {
		if model == "firefly-gpt-image" {
			ratio = "auto"
		} else {
			ratio = "1x1"
		}
	} else {
		ratio = strings.ReplaceAll(ratio, ":", "x")
	}
	if !ratios[ratio] {
		return "", fmt.Errorf("模型 %s 不支持宽高比 %s", model, ratio)
	}
	return model + "-" + resolution + "-" + ratio, nil
}

func parseFireflyImageResponse(body []byte, baseURL string) (string, error) {
	var content strings.Builder
	lines := strings.Split(string(body), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "data: [DONE]" {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var chunk struct {
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
				} `json:"delta"`
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(line), &chunk) != nil {
			continue
		}
		if chunk.Error != nil && chunk.Error.Message != "" {
			return "", fmt.Errorf("Firefly 生成失败: %s", chunk.Error.Message)
		}
		for _, choice := range chunk.Choices {
			if strings.Contains(choice.Delta.ReasoningContent, "❌") {
				return "", fmt.Errorf("Firefly 生成失败: %s", choice.Delta.ReasoningContent)
			}
			content.WriteString(choice.Delta.Content)
			content.WriteString(choice.Message.Content)
		}
	}
	if imageURL := extractImageURL(content.String(), baseURL); imageURL != "" {
		return imageURL, nil
	}
	return "", fmt.Errorf("Firefly 响应中未找到图片地址")
}

func extractImageURL(content, baseURL string) string {
	for _, pattern := range []*regexp.Regexp{markdownImagePattern, dataImagePattern, absoluteImagePattern, relativeImagePattern} {
		match := pattern.FindStringSubmatch(content)
		if len(match) == 0 {
			continue
		}
		value := match[0]
		if pattern == markdownImagePattern {
			value = match[1]
		}
		if strings.HasPrefix(value, "/") {
			if parsed, err := url.Parse(baseURL); err == nil {
				return parsed.Scheme + "://" + parsed.Host + value
			}
		}
		return value
	}
	return ""
}

func stripDataURLPrefix(value string) string {
	if strings.HasPrefix(value, "data:") {
		if comma := strings.IndexByte(value, ','); comma >= 0 {
			return value[comma+1:]
		}
	}
	return value
}

func stringSet(values ...string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}
