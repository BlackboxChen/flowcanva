package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

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
	if req.Image != "" {
		payload["image"] = []string{stripDataURLPrefix(req.Image)}
	}
	h.proxyImageJSON(c, baseURL+"/images/generations", apiKey, payload)
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
