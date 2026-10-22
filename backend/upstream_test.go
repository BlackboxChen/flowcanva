package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestResolveUpstreamUsesNodePair(t *testing.T) {
	h := &Handler{cfg: Config{ZhengAPIBaseURL: "https://zhengapi.top", ZhengAPIAPIKey: "global-key"}}
	baseURL, apiKey, err := h.resolveUpstream("https://node.example/v1", "node-key", "https://api.openai.com/v1", true)
	if err != nil || baseURL != "https://node.example/v1" || apiKey != "node-key" {
		t.Fatalf("unexpected node upstream: base=%q key=%q err=%v", baseURL, apiKey, err)
	}
}

func TestResolveUpstreamUsesZhengAPIPair(t *testing.T) {
	h := &Handler{cfg: Config{ZhengAPIBaseURL: "https://zhengapi.top/", ZhengAPIAPIKey: "global-key"}}
	baseURL, apiKey, err := h.resolveUpstream("https://other.example/v1", "", "https://api.openai.com/v1", true)
	if err != nil || baseURL != "https://zhengapi.top/v1" || apiKey != "global-key" {
		t.Fatalf("unexpected global upstream: base=%q key=%q err=%v", baseURL, apiKey, err)
	}
}

func TestNormalizeZhengAPIBaseURLForVideo(t *testing.T) {
	if got := normalizeZhengAPIBaseURL("https://zhengapi.top/v1/", false); got != "https://zhengapi.top" {
		t.Fatalf("unexpected video base URL: %q", got)
	}
}

func TestComfyVideoDimensionsAndFrames(t *testing.T) {
	w, h := comfyVideoDimensions("16:9", "480p")
	if w != 736 || h != 416 {
		t.Fatalf("unexpected 480p 16:9 dimensions: %dx%d", w, h)
	}
	w, h = comfyVideoDimensions("9:16", "720p")
	if w != 704 || h != 1248 {
		t.Fatalf("unexpected 720p 9:16 dimensions: %dx%d", w, h)
	}
	if got := comfyVideoFrames("5"); got != 124 {
		t.Fatalf("unexpected H3 frame count for 5s: %d", got)
	}
}

func TestUploadToImageHostReturnsDataURLWhenEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{cfg: Config{ZhengAPIUseDataURLs: true}}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/upload-image-host", strings.NewReader(`{"data_url":"data:image/png;base64,dGVzdA=="}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	h.UploadToImageHost(ctx)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "data:image/png;base64,dGVzdA==") {
		t.Fatalf("unexpected response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
