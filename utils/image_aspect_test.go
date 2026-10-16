package utils

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"strings"
	"testing"
	"time"
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

func TestSpecialDaysInMonth(t *testing.T) {
	loc := time.UTC
	may := SpecialDaysInMonth(2026, time.May, loc)
	want := map[string]int{"Anneler Günü": 10, "Kurban Bayramı": 27, "19 Mayıs Atatürk'ü Anma, Gençlik ve Spor Bayramı": 19}
	got := map[string]int{}
	for _, d := range may {
		got[d.Name] = d.Date.Day()
	}
	for name, dayNum := range want {
		if got[name] != dayNum {
			t.Errorf("%s: got %d want %d", name, got[name], dayNum)
		}
	}
	for _, d := range SpecialDaysInMonth(2026, time.November, loc) {
		if d.Name == "Black Friday" && d.Date.Day() != 27 {
			t.Errorf("Black Friday 2026: got %d want 27", d.Date.Day())
		}
	}
	if june := SpecialDaysInMonth(2026, time.June, loc); len(june) != 1 || june[0].Date.Day() != 21 {
		t.Errorf("Babalar Günü 2026 should be 21 June, got %+v", june)
	}
}

func TestFixAIJSONRawNewlines(t *testing.T) {
	in := "[{\"title\":\"A\",\"description\":\"Kanca?\n\nDeğer paragrafı.\n\nDM'den yaz!\"},{\"title\":\"B\",\"description\":\"x\"}]"
	out := fixAIJSON(in)
	var res []IdeaResult
	if err := json.Unmarshal([]byte(out), &res); err != nil || len(res) != 2 {
		t.Fatalf("parse failed: %v (%d)", err, len(res))
	}
	if res[0].Description != "Kanca?\n\nDeğer paragrafı.\n\nDM'den yaz!" {
		t.Errorf("description changed: %q", res[0].Description)
	}
}

func TestAdaptChatBody(t *testing.T) {
	old := []byte(`{"model":"gpt-4o-mini","temperature":0.7,"max_tokens":500,"messages":[]}`)
	if string(AdaptChatBody(old)) != string(old) {
		t.Error("non-reasoning model body must be unchanged")
	}
	var m map[string]interface{}
	json.Unmarshal(AdaptChatBody([]byte(`{"model":"gpt-5.4-mini","temperature":0.7,"max_tokens":500,"messages":[{"role":"user","content":"x"}]}`)), &m)
	if _, ok := m["temperature"]; ok {
		t.Error("temperature should be removed")
	}
	if _, ok := m["max_tokens"]; ok {
		t.Error("max_tokens should be removed")
	}
	if m["max_completion_tokens"].(float64) != 4500 || m["reasoning_effort"] != "low" || m["messages"] == nil {
		t.Errorf("unexpected body: %v", m)
	}
}

func TestAvoidSubject(t *testing.T) {
	sub := &layoutBox{X: 33, Y: 20, W: 34, H: 52}
	ls := []DesignLayer{
		{Type: "description", Text: "Taze çekirdek, el yapımı kruvasan.", X: 8, Y: 34, MaxWidth: 55, FontSize: 15, LineHeight: 1.3},
		{Type: "headline", Text: "Başlık", X: 8, Y: 5, MaxWidth: 60, FontSize: 38, LineHeight: 1.1},
	}
	out := avoidSubject(ls, sub, "4:5")
	if out[0].X+out[0].MaxWidth > sub.X && out[0].X < sub.X+sub.W {
		t.Errorf("description still overlaps subject: x=%.1f w=%.1f", out[0].X, out[0].MaxWidth)
	}
	if out[1].MaxWidth != 60 {
		t.Errorf("headline above subject should be untouched, got w=%.1f", out[1].MaxWidth)
	}
	if !isLightHex("#ffffff") || isLightHex("#222222") {
		t.Error("isLightHex wrong")
	}
}

func TestWrapCaption(t *testing.T) {
	got := wrapCaption("Güne iyi bir kahveyle başlayın bugün", 18)
	for _, line := range strings.Split(got, "\n") {
		if len([]rune(line)) > 18 {
			t.Errorf("line too long: %q", line)
		}
	}
	if strings.ReplaceAll(got, "\n", " ") != "Güne iyi bir kahveyle başlayın bugün" {
		t.Errorf("words changed: %q", got)
	}
}

func TestSpeakableText(t *testing.T) {
	got := SpeakableText("İNKA İK ile KOBİ ve SEO işleri. Bordro hazır.", nil)
	want := "İnka i ka ile kobi ve seo işleri. Bordro hazır."
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
	if got := SpeakableText("DIO ile", ParsePronunciations("DIO=Dio")); got != "Dio ile" {
		t.Errorf("override: %q", got)
	}
}
