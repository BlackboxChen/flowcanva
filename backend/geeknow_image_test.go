package main

import "testing"

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
