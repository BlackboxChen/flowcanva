package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildFireflyModelID(t *testing.T) {
	tests := []struct {
		model      string
		resolution string
		ratio      string
		want       string
	}{
		{"firefly-nano-banana", "2K", "16:9", "firefly-nano-banana-2k-16x9"},
		{"firefly-nano-banana-pro", "4K", "1:1", "firefly-nano-banana-pro-4k-1x1"},
		{"firefly-nano-banana2", "1K", "1:8", "firefly-nano-banana2-1k-1x8"},
		{"firefly-gpt-image", "2K", "自适应", "firefly-gpt-image-2k-auto"},
	}
	for _, test := range tests {
		got, err := buildFireflyModelID(test.model, test.resolution, test.ratio)
		if err != nil || got != test.want {
			t.Errorf("buildFireflyModelID(%q): got=%q err=%v want=%q", test.model, got, err, test.want)
		}
	}
}

func TestBuildFireflyModelIDRejectsUnsupportedRatio(t *testing.T) {
	if _, err := buildFireflyModelID("firefly-nano-banana", "2K", "2:3"); err == nil {
		t.Fatal("expected unsupported ratio error")
	}
}

func TestParseFireflyImageResponse(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"![image](https://cdn.example.com/ima"}}]}`,
		`data: {"choices":[{"delta":{"content":"ges/result.png)"}}]}`,
		`data: [DONE]`,
	}, "\n")
	got, err := parseFireflyImageResponse([]byte(body), "https://zhengapi.top/v1")
	if err != nil || got != "https://cdn.example.com/images/result.png" {
		t.Fatalf("unexpected parsed URL: got=%q err=%v", got, err)
	}
}

func TestParseFireflyRelativeImageURL(t *testing.T) {
	body := `data: {"choices":[{"delta":{"content":"/v1/files/result.png"}}]}`
	got, err := parseFireflyImageResponse([]byte(body), "https://zhengapi.top/v1")
	if err != nil || got != "https://zhengapi.top/v1/files/result.png" {
		t.Fatalf("unexpected relative URL: got=%q err=%v", got, err)
	}
}

func TestStripDataURLPrefix(t *testing.T) {
	if got := stripDataURLPrefix("data:image/jpeg;base64,/9j/test"); got != "/9j/test" {
		t.Fatalf("unexpected stripped image: %q", got)
	}
}

func TestMergeZhengAPIImageModelsIsIdempotent(t *testing.T) {
	raw := `{"models":[{"name":"custom-image","channel":"Custom"}],"keep":true}`
	first, added, err := mergeZhengAPIImageModels(raw)
	if err != nil || added != len(zhengAPIImageModels) {
		t.Fatalf("first merge: added=%d err=%v", added, err)
	}
	second, addedAgain, err := mergeZhengAPIImageModels(first)
	if err != nil || addedAgain != 0 {
		t.Fatalf("second merge: added=%d err=%v", addedAgain, err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(second), &parsed); err != nil || parsed["keep"] != true {
		t.Fatalf("custom config was not preserved: %s", second)
	}
}
