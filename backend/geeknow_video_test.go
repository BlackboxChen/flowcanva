package main

import "testing"

func TestBuildGeekNowVideoPayloadUsesWANReferenceMediaTypes(t *testing.T) {
	payload, err := buildGeekNowVideoPayload(VideoGenRequest{
		Model:      "wan3.0-video",
		Prompt:     "让图1中的人物比心",
		Seconds:    "5",
		Ratio:      "16:9",
		Resolution: "480p",
		ImageURLs:  []string{"https://example.com/reference.jpg"},
		VideoURLs:  []string{"https://example.com/reference.mp4"},
		AudioURLs:  []string{"https://example.com/reference.mp3"},
	})
	if err != nil {
		t.Fatal(err)
	}

	media := payload["media"].([]map[string]string)
	wantTypes := []string{"reference_image", "reference_video", "reference_audio"}
	for index, want := range wantTypes {
		if media[index]["type"] != want || media[index]["url"] == "" {
			t.Fatalf("unexpected media[%d]: %#v", index, media[index])
		}
	}
}

func TestBuildGeekNowVideoPayloadKeepsSingleImageURLFirst(t *testing.T) {
	payload, err := buildGeekNowVideoPayload(VideoGenRequest{
		Model:     "wan3.0-video",
		ImageURL:  "https://example.com/first.jpg",
		ImageURLs: []string{"https://example.com/second.jpg"},
	})
	if err != nil {
		t.Fatal(err)
	}

	media := payload["media"].([]map[string]string)
	if len(media) != 2 || media[0]["url"] != "https://example.com/first.jpg" || media[1]["url"] != "https://example.com/second.jpg" {
		t.Fatalf("unexpected media order: %#v", media)
	}
}

func TestBuildGeekNowVideoPayloadNormalizesCanvasMentions(t *testing.T) {
	payload, err := buildGeekNowVideoPayload(VideoGenRequest{
		Model:  "wan3.0-video",
		Prompt: "@图片1 比心，并使用 @视频1 的运镜和 @音频1 的声音",
	})
	if err != nil {
		t.Fatal(err)
	}

	want := "图1 比心，并使用 视频1 的运镜和 音频1 的声音"
	if payload["prompt"] != want {
		t.Fatalf("unexpected prompt: got=%q want=%q", payload["prompt"], want)
	}
}
