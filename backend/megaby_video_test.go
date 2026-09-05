package main

import (
	"encoding/json"
	"testing"
)

func TestBuildMegaBYVideoPayloadUsesPublicModelAndDocumentFields(t *testing.T) {
	payload, err := buildMegaBYVideoPayload(VideoGenRequest{
		Model: "megaby-alibaba/wan-3.0", Prompt: "a test", Seconds: "8", Ratio: "16:9", Resolution: "720p",
		ImageURLs: []string{"https://example.com/a.jpg"}, VideoURLs: []string{"https://example.com/a.mp4"}, AudioURLs: []string{"https://example.com/a.mp3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if payload["model"] != "alibaba/wan-3.0" || payload["duration"] != 8 {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if _, ok := payload["referenceImages"]; !ok {
		t.Fatal("referenceImages missing")
	}
	if _, ok := payload["referenceVideos"]; !ok {
		t.Fatal("referenceVideos missing")
	}
	if _, ok := payload["referenceAudios"]; !ok {
		t.Fatal("referenceAudios missing")
	}
}

func TestMergeMegaBYVideoModelsRemovesOtherMegaBYModels(t *testing.T) {
	raw := `{"models":[{"name":"megaby-videos-mini","channel":"MegaBY"},{"name":"sd-2.0-15s-16x9-480p","channel":"ZhengAPI"}]}`
	merged, _, err := mergeMegaBYVideoModels(raw)
	if err != nil {
		t.Fatal(err)
	}
	var extra map[string]any
	if err := json.Unmarshal([]byte(merged), &extra); err != nil {
		t.Fatal(err)
	}
	models := extra["models"].([]any)
	if len(models) != len(megaBYVideoModels)+1 {
		t.Fatalf("unexpected models: %#v", models)
	}
	for _, item := range models {
		name := item.(map[string]any)["name"]
		if name == "megaby-videos-mini" {
			t.Fatalf("legacy MegaBY model was retained")
		}
	}
}

func TestMergeMegaBYVideoModelsIsIdempotent(t *testing.T) {
	first, added, err := mergeMegaBYVideoModels(`{"models":[]}`)
	if err != nil || added != len(megaBYVideoModels) {
		t.Fatalf("first merge: added=%d err=%v", added, err)
	}
	second, addedAgain, err := mergeMegaBYVideoModels(first)
	if err != nil || addedAgain != 0 {
		t.Fatalf("second merge: added=%d err=%v", addedAgain, err)
	}
	var extra map[string]any
	if err := json.Unmarshal([]byte(second), &extra); err != nil {
		t.Fatal(err)
	}
	if len(extra["models"].([]any)) != len(megaBYVideoModels) {
		t.Fatalf("unexpected models: %#v", extra["models"])
	}
}
