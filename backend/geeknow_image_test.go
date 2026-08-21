package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGeekNowImageSizes(t *testing.T) {
	tests := []struct{ model, resolution, ratio, want string }{
		{"gpt-image-2", "1K", "16:9", "1920x1080"},
		{"gpt-image-2-pro", "2K", "3:2", "2560x1712"},
		{"gpt-image-2-pro", "4K", "9:16", "2160x3840"},
		{"gpt-image-2-vip", "1K", "21:9", "1344x576"},
		{"gpt-image-2-vip", "2K", "3:4", "1728x2304"},
		{"grok-4-2-image", "2K", "16:9", "2560x1440"},
	}
	for _, test := range tests {
		got, err := geekNowImageSize(test.model, test.resolution, test.ratio)
		if err != nil || got != test.want {
			t.Errorf("%s %s %s: got=%q err=%v want=%q", test.model, test.resolution, test.ratio, got, err, test.want)
		}
	}
}

func TestGeekNowImageSizeRejectsUnsupportedRatio(t *testing.T) {
	if _, err := geekNowImageSize("gpt-image-2", "1K", "21:9"); err == nil {
		t.Fatal("expected unsupported ratio error")
	}
}

func TestGenerateGeekNowImagePreservesAllReferences(t *testing.T) {
	var payload map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"url":"https://cdn.example/result.png"}]}`)
	}))
	defer upstream.Close()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/images/generate", strings.NewReader(`{}`))
	h := &Handler{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	h.generateGeekNowImage(ctx, ImageGenRequest{
		Model:       "gpt-image-2",
		Prompt:      "combine references",
		AspectRatio: "1:1",
		ImageSize:   "1K",
		Image:       "legacy-reference",
		Images: []string{
			"data:image/png;base64,first-reference",
			"https://cdn.example/second.png",
			"third-reference",
		},
	}, upstream.URL+"/v1", "test-key")

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected response: code=%d body=%s", recorder.Code, recorder.Body.String())
	}
	images, ok := payload["image"].([]any)
	if !ok || len(images) != 3 {
		t.Fatalf("references were not preserved: %#v", payload["image"])
	}
	want := []string{"first-reference", "https://cdn.example/second.png", "third-reference"}
	for i, value := range want {
		if images[i] != value {
			t.Fatalf("reference %d: got=%#v want=%q", i, images[i], value)
		}
	}
}
