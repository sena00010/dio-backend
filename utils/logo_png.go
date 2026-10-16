package utils

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"os/exec"
	"strings"
)

// PrepareLogoPNG — marka logosunu video için şeffaf zeminli PNG'ye çevirir (en fazla maxSide px).
// SVG logolar rsvg-convert ile rasterleştirilir. Düz zemin (çoğunlukla beyaz) ve harf içi boşluklar
// şeffaflaştırılır; geniş renk alanı içindeki bilinçli beyazlar korunur (istemcideki algoritmanın aynısı).
func PrepareLogoPNG(logoURL, outPath string, maxSide int) error {
	if strings.TrimSpace(logoURL) == "" {
		return fmt.Errorf("logo yok")
	}
	raw := outPath + ".src"
	if err := downloadTo(logoURL, raw); err != nil {
		return err
	}
	defer os.Remove(raw)

	data, err := os.ReadFile(raw)
	if err != nil {
		return err
	}
	if bytes.Contains(data[:min(len(data), 512)], []byte("<svg")) || strings.HasSuffix(strings.ToLower(strings.Split(logoURL, "?")[0]), ".svg") {
		if _, err := exec.LookPath("rsvg-convert"); err != nil {
			return fmt.Errorf("SVG logo için rsvg-convert yok")
		}
		out, err := exec.Command("rsvg-convert", "-w", fmt.Sprint(maxSide), "-a", raw).Output()
		if err != nil {
			return fmt.Errorf("SVG logo dönüştürülemedi: %v", err)
		}
		data = out
	}

	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("logo okunamadı: %v", err)
	}
	img := toRGBA(src, maxSide)
	makeBackgroundTransparent(img)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	return os.WriteFile(outPath, buf.Bytes(), 0o644)
}

// toRGBA — görseli RGBA'ya çevirir, uzun kenarı maxSide'ı aşıyorsa basit örnekleme ile küçültür
func toRGBA(src image.Image, maxSide int) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	scale := math.Min(1, float64(maxSide)/math.Max(float64(w), float64(h)))
	nw, nh := int(math.Max(1, math.Round(float64(w)*scale))), int(math.Max(1, math.Round(float64(h)*scale)))
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	if scale == 1 {
		draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
		return dst
	}
	for y := 0; y < nh; y++ {
		for x := 0; x < nw; x++ {
			sx := b.Min.X + int(float64(x)/scale)
			sy := b.Min.Y + int(float64(y)/scale)
			dst.Set(x, y, color.RGBAModel.Convert(src.At(sx, sy)))
		}
	}
	return dst
}

func makeBackgroundTransparent(img *image.RGBA) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	px := img.Pix
	idx := func(p int) int { return (p/w)*img.Stride + (p%w)*4 }
	corners := []int{idx(0), idx(w - 1), idx((h - 1) * w), idx(h*w - 1)}
	allClear := true
	for _, c := range corners {
		if px[c+3] >= 20 {
			allClear = false
		}
	}
	if allClear {
		return // zaten şeffaf
	}
	var bg [3]float64
	for c := 0; c < 3; c++ {
		for _, i := range corners {
			bg[c] += float64(px[i+c]) / 4
		}
	}
	dist := func(i int) float64 {
		return math.Abs(float64(px[i])-bg[0]) + math.Abs(float64(px[i+1])-bg[1]) + math.Abs(float64(px[i+2])-bg[2])
	}
	for _, c := range corners {
		if dist(c) > 60 {
			return // düz zemin yok
		}
	}
	const tol = 48.0
	clear := func(i int) {
		d := dist(i)
		if d < tol*0.5 {
			px[i+3] = 0
		} else {
			px[i+3] = uint8(float64(px[i+3]) * ((d - tol*0.5) / (tol * 0.5)))
		}
	}

	outside := make([]bool, w*h)
	stack := make([]int, 0, 2*(w+h))
	for x := 0; x < w; x++ {
		stack = append(stack, x, (h-1)*w+x)
	}
	for y := 0; y < h; y++ {
		stack = append(stack, y*w, y*w+w-1)
	}
	visited := make([]bool, w*h)
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if visited[p] {
			continue
		}
		visited[p] = true
		i := idx(p)
		if dist(i) > tol {
			continue
		}
		outside[p] = true
		clear(i)
		x := p % w
		if x > 0 {
			stack = append(stack, p-1)
		}
		if x < w-1 {
			stack = append(stack, p+1)
		}
		if p >= w {
			stack = append(stack, p-w)
		}
		if p < w*(h-1) {
			stack = append(stack, p+w)
		}
	}

	// Harf içi boşluklar: ince çizgiyle çevrili kapalı zemin bölgeleri
	reach := int(math.Max(4, math.Round(math.Min(float64(w), float64(h))*0.15)))
	seen := make([]bool, w*h)
	dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}}
	for start := 0; start < w*h; start++ {
		if seen[start] || outside[start] || dist(idx(start)) > tol {
			continue
		}
		region := []int{}
		in := map[int]bool{}
		q := []int{start}
		seen[start] = true
		for len(q) > 0 {
			p := q[len(q)-1]
			q = q[:len(q)-1]
			region = append(region, p)
			in[p] = true
			x := p % w
			for _, nb := range []int{p - 1, p + 1, p - w, p + w} {
				if nb < 0 || nb >= w*h || (nb == p-1 && x == 0) || (nb == p+1 && x == w-1) {
					continue
				}
				if seen[nb] || outside[nb] || dist(idx(nb)) > tol {
					continue
				}
				seen[nb] = true
				q = append(q, nb)
			}
		}
		if len(region) > w*h*8/100 {
			continue
		}
		boundary := region[:0:0]
		for _, p := range region {
			x := p % w
			if (x > 0 && !in[p-1]) || (x < w-1 && !in[p+1]) || !in[p-w] || !in[p+w] {
				boundary = append(boundary, p)
			}
		}
		samples, hits := 0, 0
		step := int(math.Max(1, float64(len(boundary))/120))
		for k := 0; k < len(boundary); k += step {
			bx, by := boundary[k]%w, boundary[k]/w
			samples++
			hit := false
			for _, d := range dirs {
				for s := 1; s <= reach && !hit; s++ {
					x, y := bx+d[0]*s, by+d[1]*s
					if x < 0 || y < 0 || x >= w || y >= h {
						break
					}
					qq := y*w + x
					if in[qq] {
						continue
					}
					if outside[qq] {
						hit = true
					}
				}
				if hit {
					break
				}
			}
			if hit {
				hits++
			}
		}
		if samples > 0 && float64(hits)/float64(samples) >= 0.6 {
			for _, p := range region {
				clear(idx(p))
			}
		}
	}
}
