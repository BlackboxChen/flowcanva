package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildZhengVideoPayloadPreservesReferenceTypesAndDataURLs(t *testing.T) {
	payload, err := buildZhengVideoPayload(VideoGenRequest{
		Model: "sd-2.0-sec-480p", Prompt: "test",
		ImageURLs: []string{"data:image/png;base64,aW1hZ2U="},
		VideoURLs: []string{"data:video/mp4;base64,dmlkZW8="},
		AudioURLs: []string{"data:audio/wav;base64,YXVkaW8="},
	})
	if err != nil {
		t.Fatal(err)
	}
	if payload["seconds"] != "15" || payload["size"] != "854x480" {
		t.Fatalf("unexpected specs: %#v", payload)
	}
	input := payload["input"].(map[string]any)
	media := input["media"].([]map[string]string)
	wantTypes := []string{"reference_image", "reference_video", "reference_audio"}
	for i, want := range wantTypes {
		if media[i]["type"] != want || media[i]["url"] == "" {
			t.Fatalf("unexpected media[%d]: %#v", i, media[i])
		}
	}
}

func TestMergeZhengAPIVideoModelsIsIdempotent(t *testing.T) {
	first, added, err := mergeZhengAPIVideoModels(`{"models":[{"name":"custom-video","channel":"Custom"}]}`)
	if err != nil || added != len(zhengAPIVideoModels) {
		t.Fatalf("first merge: added=%d err=%v", added, err)
	}
	second, addedAgain, err := mergeZhengAPIVideoModels(first)
	if err != nil || addedAgain != 0 {
		t.Fatalf("second merge: added=%d err=%v", addedAgain, err)
	}
	var extra map[string]any
	if err := json.Unmarshal([]byte(second), &extra); err != nil {
		t.Fatal(err)
	}
	if len(extra["models"].([]any)) != len(zhengAPIVideoModels)+1 {
		t.Fatalf("unexpected models: %#v", extra["models"])
	}
}

func TestMergeZhengAPIVideoModelsRemovesRetiredPresets(t *testing.T) {
	raw := `{"models":[{"name":"sd-2.0-15s-16x9-720p","channel":"ZhengAPI"},{"name":"sd-2.0-15s-9x16-1080p","channel":"ZhengAPI"},{"name":"sd-2.0fast-15s-16x9-480p","channel":"ZhengAPI"},{"name":"keep-me","channel":"Custom"}]}`
	merged, _, err := mergeZhengAPIVideoModels(raw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(merged, "720p") || strings.Contains(merged, "1080p") || strings.Contains(merged, "fast") || !strings.Contains(merged, "keep-me") || !strings.Contains(merged, "sd-2.0-sec-480p") {
		t.Fatalf("retired or new models not handled: %s", merged)
	}
}

func TestZhengVideoSize(t *testing.T) {
	cases := map[string]string{
		"sd-2.0-15s-16x9-480p": "854x480",
		"sd-2.0-sec-480p":      "854x480",
	}
	for model, want := range cases {
		if got := zhengVideoSize(model); got != want {
			t.Fatalf("%s: got %s want %s", model, got, want)
		}
	}
}
