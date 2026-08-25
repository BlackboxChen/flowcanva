package main

import (
	"encoding/json"
	"testing"
)

func TestBuildMegaBYVideoPayloadUsesPublicModelAndDocumentFields(t *testing.T) {
	payload, err := buildMegaBYVideoPayload(VideoGenRequest{
		Model: "megaby-videos-mini", Prompt: "a test", Seconds: "8", Ratio: "16:9", Resolution: "720p",
		ImageURLs: []string{"https://example.com/a.jpg"}, VideoURLs: []string{"https://example.com/a.mp4"}, AudioURLs: []string{"https://example.com/a.mp3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if payload["model"] != "videos-mini" || payload["duration"] != 8 {
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
