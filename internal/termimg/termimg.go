// Package termimg draws images with truecolor half-block characters. Each
// terminal cell shows two vertical pixels, which works in any truecolor
// terminal (Alacritty included) without a graphics protocol.
package termimg

import (
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"

	"golang.org/x/image/draw"
)

// Render loads path and returns it as rows of ANSI text fitting in
// cols x rows cells, preserving aspect ratio.
func Render(path string, cols, rows int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return "", err
	}

	b := src.Bounds()
	w, h := cols, rows*2
	// Terminal cells are ~1:2, so one cell column ≈ one half-block row.
	if b.Dx()*h > b.Dy()*w {
		h = max(2, b.Dy()*w/b.Dx())
	} else {
		w = max(1, b.Dx()*h/b.Dy())
	}
	h += h % 2
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)

	var sb strings.Builder
	for y := 0; y < h; y += 2 {
		for x := 0; x < w; x++ {
			top, bot := dst.NRGBAAt(x, y), dst.NRGBAAt(x, y+1)
			tOn, bOn := top.A >= 96, bot.A >= 96
			switch {
			case tOn && bOn:
				fmt.Fprintf(&sb, "\x1b[38;2;%sm\x1b[48;2;%sm▀", rgb(top), rgb(bot))
			case tOn:
				fmt.Fprintf(&sb, "\x1b[38;2;%sm▀", rgb(top))
			case bOn:
				fmt.Fprintf(&sb, "\x1b[38;2;%sm▄", rgb(bot))
			default:
				sb.WriteByte(' ')
			}
			sb.WriteString("\x1b[0m")
		}
		if y+2 < h {
			sb.WriteByte('\n')
		}
	}
	return sb.String(), nil
}

func rgb(c color.NRGBA) string { return fmt.Sprintf("%d;%d;%d", c.R, c.G, c.B) }
