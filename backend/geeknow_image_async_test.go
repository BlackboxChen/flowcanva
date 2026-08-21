package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestGeekNowImageAsyncCreateAndPoll(t *testing.T) {
	var polls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("missing authorization")
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/images/generations/async":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"task_img_test","status":"queued"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/images/generations/async/task_img_test":
			w.Header().Set("Content-Type", "application/json")
			polls.Add(1)
			_, _ = io.WriteString(w, `{"data":[{"url":"https://cdn.example/result.png"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	oldInterval, oldWait := geekNowImagePollInterval, geekNowImageMaxWait
	geekNowImagePollInterval, geekNowImageMaxWait = time.Millisecond, time.Second
	defer func() {
		geekNowImagePollInterval, geekNowImageMaxWait = oldInterval, oldWait
	}()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/images/generate", strings.NewReader(`{}`))
	h := &Handler{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	h.generateGeekNowImageAsync(ctx, upstream.URL+"/v1", "test-key", map[string]any{"model": "gpt-image-2"})

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "https://cdn.example/result.png") {
		t.Fatalf("unexpected response: code=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if polls.Load() != 1 {
		t.Fatalf("unexpected poll count: %d", polls.Load())
	}
}

func TestGeekNowImageAsyncFailureMessage(t *testing.T) {
	data := map[string]any{"error": map[string]any{"message": "generation failed"}}
	if got := geekNowImageError(data); got != "generation failed" {
		t.Fatalf("unexpected error message: %q", got)
	}
}
