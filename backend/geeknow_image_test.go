package main

import (
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGeekNowImageSizes(t *testing.T) {
	tests := []struct{ model, resolution, ratio, want string }{
		{"gpt-image-2", "1K", "16:9", "1920x1080"},
		{"gpt-image-2.5-flare", "1K", "16:9", "1920x1080"},
		{"gpt-image-2.5-sunburst", "1K", "1:1", "1024x1024"},
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
	const firstImage = "first-reference"
	const secondImage = "second-reference"
	referenceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = io.WriteString(w, secondImage)
	}))
	defer referenceServer.Close()

	workdir := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workdir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	var fields map[string]string
	var images []string
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/result.png" {
			w.Header().Set("Content-Type", "image/png")
			_, _ = io.WriteString(w, "generated-image")
			return
		}
		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data;") {
			t.Fatalf("unexpected content type: %s", r.Header.Get("Content-Type"))
		}
		reader, err := r.MultipartReader()
		if err != nil {
			t.Fatal(err)
		}
		fields, images = readImageEditParts(t, reader)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"url":"`+upstream.URL+`/result.png"}]}`)
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
		Images: []string{
			"data:image/png;base64,Zmlyc3QtcmVmZXJlbmNl",
			referenceServer.URL + "/second.png",
		},
	}, upstream.URL+"/v1", "test-key")

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected response: code=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "/uploads/generated/") || strings.Contains(recorder.Body.String(), upstream.URL) {
		t.Fatalf("result was not localized: %s", recorder.Body.String())
	}
	if fields["model"] != "gpt-image-2" || fields["prompt"] != "combine references" || fields["size"] != "1024x1024" {
		t.Fatalf("unexpected fields: %#v", fields)
	}
	if len(images) != 2 {
		t.Fatalf("unexpected image count: %d", len(images))
	}
	want := []string{firstImage, secondImage}
	for i, value := range want {
		if images[i] != value {
			t.Fatalf("reference %d: got=%q want=%q", i, images[i], value)
		}
	}
}

func readImageEditParts(t *testing.T, reader *multipart.Reader) (map[string]string, []string) {
	t.Helper()
	fields := map[string]string{}
	var images []string
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(part)
		if err != nil {
			t.Fatal(err)
		}
		if part.FormName() == "image[]" {
			images = append(images, string(data))
		} else {
			fields[part.FormName()] = string(data)
		}
	}
	return fields, images
}

func TestGenerateGeekNowImageRejectsTooManyReferences(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/images/generate", strings.NewReader(`{}`))
	h := &Handler{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	references := make([]string, maxImageReferences+1)
	for i := range references {
		references[i] = "data:image/png;base64,WA=="
	}
	h.generateGeekNowImage(ctx, ImageGenRequest{
		Model: "gpt-image-2", Prompt: "test", AspectRatio: "1:1", ImageSize: "1K", Images: references,
	}, "http://unused", "test-key")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unexpected response: code=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
