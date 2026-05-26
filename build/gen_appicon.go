//go:build ignore

package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

const size = 512

func main() {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))

	// Dark navy gradient background, rounded square corners.
	bgTop := color.NRGBA{R: 0x12, G: 0x16, B: 0x2B, A: 0xFF}
	bgBot := color.NRGBA{R: 0x05, G: 0x07, B: 0x10, A: 0xFF}
	radius := float64(size) * 0.22

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if !insideRoundedRect(float64(x), float64(y), 0, 0, size, size, radius) {
				continue
			}
			t := float64(y) / float64(size)
			r := lerp(float64(bgTop.R), float64(bgBot.R), t)
			g := lerp(float64(bgTop.G), float64(bgBot.G), t)
			b := lerp(float64(bgTop.B), float64(bgBot.B), t)
			img.Set(x, y, color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 0xFF})
		}
	}

	// Soft halo glow behind the star.
	cx, cy := float64(size)/2, float64(size)/2
	haloR := float64(size) * 0.40
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			d := math.Hypot(float64(x)-cx, float64(y)-cy)
			if d > haloR {
				continue
			}
			intensity := math.Pow(1.0-d/haloR, 2.5)
			if intensity <= 0 {
				continue
			}
			if !insideRoundedRect(float64(x), float64(y), 0, 0, size, size, radius) {
				continue
			}
			existing := img.NRGBAAt(x, y)
			add := color.NRGBA{
				R: 0x6E,
				G: 0x8A,
				B: 0xFF,
				A: uint8(80 * intensity),
			}
			img.SetNRGBA(x, y, blend(existing, add))
		}
	}

	// Four-pointed star.
	// Long arms (top/bottom/left/right) + short diagonal arms for sparkle feel.
	star := color.NRGBA{R: 0xFF, G: 0xF7, B: 0xE0, A: 0xFF}
	core := color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	armLong := float64(size) * 0.38
	armShort := float64(size) * 0.14
	thick := float64(size) * 0.040

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			px, py := float64(x)-cx, float64(y)-cy

			// Cardinal arms: thin diamond along x=0 and y=0 axes.
			rxAxis := math.Abs(px)/armLong + math.Abs(py)/thick
			ryAxis := math.Abs(py)/armLong + math.Abs(px)/thick
			if rxAxis < 1.0 || ryAxis < 1.0 {
				img.SetNRGBA(x, y, blend(img.NRGBAAt(x, y), star))
				continue
			}

			// Diagonal sparkle arms (45°): thin diamond along u/v axes.
			u := (px + py) / math.Sqrt2
			v := (px - py) / math.Sqrt2
			rDiag1 := math.Abs(u)/armShort + math.Abs(v)/(thick*0.7)
			rDiag2 := math.Abs(v)/armShort + math.Abs(u)/(thick*0.7)
			if rDiag1 < 1.0 || rDiag2 < 1.0 {
				img.SetNRGBA(x, y, blend(img.NRGBAAt(x, y), star))
				continue
			}
		}
	}

	// Bright core dot.
	coreR := float64(size) * 0.055
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			d := math.Hypot(float64(x)-cx, float64(y)-cy)
			if d <= coreR {
				a := uint8(255 * math.Max(0, 1-d/coreR))
				c := core
				c.A = a
				img.SetNRGBA(x, y, blend(img.NRGBAAt(x, y), c))
			}
		}
	}

	out, err := os.Create("build/appicon.png")
	if err != nil {
		panic(err)
	}
	defer out.Close()
	if err := png.Encode(out, img); err != nil {
		panic(err)
	}
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

func insideRoundedRect(px, py, x, y, w, h, r float64) bool {
	if px < x || py < y || px >= x+w || py >= y+h {
		return false
	}
	// distance to nearest corner center
	cx := math.Min(math.Max(px, x+r), x+w-r)
	cy := math.Min(math.Max(py, y+r), y+h-r)
	d := math.Hypot(px-cx, py-cy)
	return d <= r
}

func blend(dst, src color.NRGBA) color.NRGBA {
	sa := float64(src.A) / 255
	da := float64(dst.A) / 255
	outA := sa + da*(1-sa)
	if outA == 0 {
		return color.NRGBA{}
	}
	r := (float64(src.R)*sa + float64(dst.R)*da*(1-sa)) / outA
	g := (float64(src.G)*sa + float64(dst.G)*da*(1-sa)) / outA
	b := (float64(src.B)*sa + float64(dst.B)*da*(1-sa)) / outA
	return color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: uint8(outA * 255)}
}
