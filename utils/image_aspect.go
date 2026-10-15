package utils

import (
	"bytes"
	"image"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"log"
	"math"
)

// cropToAspect — görseli ortadan hedef orana kırpar ("4:5", "9:16", "16:9", "1:1").
// Oran zaten tutuyorsa veya görsel çözülemezse orijinal byte'ları döndürür.
func cropToAspect(data []byte, ratio string) []byte {
	targets := map[string]float64{"1:1": 1, "4:5": 0.8, "9:16": 9.0 / 16.0, "16:9": 16.0 / 9.0}
	target, ok := targets[ratio]
	if !ok {
		return data
	}

	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 || math.Abs(float64(w)/float64(h)-target) < 0.01 {
		return data
	}

	cw, ch := w, h
	if float64(w)/float64(h) > target {
		cw = int(math.Round(float64(h) * target))
	} else {
		ch = int(math.Round(float64(w) / target))
	}
	x0 := b.Min.X + (w-cw)/2
	y0 := b.Min.Y + (h-ch)/2

	dst := image.NewRGBA(image.Rect(0, 0, cw, ch))
	draw.Draw(dst, dst.Bounds(), src, image.Pt(x0, y0), draw.Src)

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return data
	}
	log.Printf("✂️ Görsel %dx%d → %dx%d (%s) kırpıldı", w, h, cw, ch, ratio)
	return buf.Bytes()
}
