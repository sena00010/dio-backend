package utils

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func TestCropToAspect(t *testing.T) {
	cases := []struct {
		w, h   int
		ratio  string
		ww, wh int
	}{
		{1024, 1536, "4:5", 1024, 1280},
		{768, 1024, "4:5", 768, 960},
		{1024, 1024, "4:5", 819, 1024},
		{1080, 1350, "4:5", 1080, 1350},
		{1024, 1024, "9:16", 576, 1024},
		{1536, 1024, "16:9", 1536, 864},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, c.w, c.h)))
		out := cropToAspect(buf.Bytes(), c.ratio)
		img, _, err := image.Decode(bytes.NewReader(out))
		if err != nil {
			t.Fatal(err)
		}
		if img.Bounds().Dx() != c.ww || img.Bounds().Dy() != c.wh {
			t.Errorf("%dx%d %s: got %dx%d want %dx%d", c.w, c.h, c.ratio, img.Bounds().Dx(), img.Bounds().Dy(), c.ww, c.wh)
		}
	}
	if got := cropToAspect([]byte("not an image"), "4:5"); string(got) != "not an image" {
		t.Error("invalid input should be returned unchanged")
	}
}

func TestPlatformAspectRatio(t *testing.T) {
	for p, want := range map[string]string{"Instagram": "4:5", "Facebook": "4:5", "LinkedIn": "1:1", "TikTok": "9:16", "X": "16:9", "": "1:1"} {
		if got := PlatformAspectRatio(p); got != want {
			t.Errorf("%q: got %s want %s", p, got, want)
		}
	}
}
