package termimg

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"math"
	"math/rand/v2"
	"os"
	"strings"
	"sync"

	"golang.org/x/image/draw"
	"golang.org/x/sys/unix"
)

// Mode is how item icons are drawn.
type Mode int

const (
	Blocks Mode = iota // truecolor half-blocks, works anywhere
	Kitty              // kitty graphics protocol (kitty, Ghostty)
	Off
)

func (m Mode) String() string { return [...]string{"blocks", "kitty", "off"}[m] }

// Detect picks the best mode for the current terminal. SESHAT_IMAGES
// (kitty, blocks, off) overrides it.
func Detect() Mode {
	switch strings.ToLower(os.Getenv("SESHAT_IMAGES")) {
	case "kitty":
		return Kitty
	case "blocks":
		return Blocks
	case "off", "none":
		return Off
	}
	// tmux and screen need passthrough wrapping that Seshat doesn't do.
	if os.Getenv("TMUX") != "" || strings.HasPrefix(os.Getenv("TERM"), "screen") {
		return Blocks
	}
	term := os.Getenv("TERM")
	if os.Getenv("TERM_PROGRAM") == "ghostty" || term == "xterm-ghostty" ||
		term == "xterm-kitty" || os.Getenv("KITTY_WINDOW_ID") != "" {
		return Kitty
	}
	return Blocks
}

var (
	writeMu sync.Mutex
	idMu    sync.Mutex
	nextID  = rand.Uint32N(1<<23) + 1<<16 // random base avoids clashing with other programs
	usedIDs []uint32
)

func newID() uint32 {
	idMu.Lock()
	defer idMu.Unlock()
	id := nextID & 0xFFFFFF
	nextID++
	usedIDs = append(usedIDs, id)
	return id
}

// cellSize is the terminal's cell size in pixels, falling back to a typical
// 1:2 ratio when the terminal doesn't report it.
func cellSize() (w, h float64) {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Xpixel == 0 || ws.Ypixel == 0 || ws.Col == 0 || ws.Row == 0 {
		return 10, 20
	}
	return float64(ws.Xpixel) / float64(ws.Col), float64(ws.Ypixel) / float64(ws.Row)
}

// RenderKitty uploads the image at path to the terminal and returns text made
// of Unicode placeholders that displays it within cols x maxRows cells.
// Placeholders are ordinary text, so the image moves and redraws with the
// TUI instead of fighting its renderer.
func RenderKitty(path string, cols, maxRows int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	src, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		return "", err
	}

	cw, ch := cellSize()
	b := src.Bounds()
	aspect := float64(b.Dy()) / float64(b.Dx())
	rows := int(math.Round(float64(cols) * cw * aspect / ch))
	if rows > maxRows {
		rows = maxRows
		cols = int(math.Round(float64(rows) * ch / (cw * aspect)))
	}
	cols, rows = max(cols, 1), max(rows, 1)
	if cols > len(diacritics) || rows > len(diacritics) {
		return "", fmt.Errorf("image too large for placeholders")
	}

	// Send only the pixels the cells can show.
	img := src
	if pw, ph := int(float64(cols)*cw), int(float64(rows)*ch); pw < b.Dx() {
		dst := image.NewNRGBA(image.Rect(0, 0, pw, ph))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
		img = dst
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}

	id := newID()
	if err := transmit(id, cols, rows, buf.Bytes()); err != nil {
		return "", err
	}
	return placeholders(id, cols, rows), nil
}

// transmit sends the PNG in 4 KiB chunks, then creates a virtual placement
// (U=1) that the placeholder cells will reference. q=2 keeps the terminal
// from replying, since replies would arrive as keystrokes.
func transmit(id uint32, cols, rows int, data []byte) error {
	enc := base64.StdEncoding.EncodeToString(data)
	var out strings.Builder
	for i := 0; i < len(enc); i += 4096 {
		end := min(i+4096, len(enc))
		more := 1
		if end == len(enc) {
			more = 0
		}
		if i == 0 {
			fmt.Fprintf(&out, "\x1b_Ga=t,f=100,t=d,q=2,i=%d,m=%d;%s\x1b\\", id, more, enc[i:end])
		} else {
			fmt.Fprintf(&out, "\x1b_Gm=%d;%s\x1b\\", more, enc[i:end])
		}
	}
	fmt.Fprintf(&out, "\x1b_Ga=p,U=1,q=2,i=%d,c=%d,r=%d\x1b\\", id, cols, rows)

	// One write call: the kernel won't interleave it with the TUI's frames.
	writeMu.Lock()
	defer writeMu.Unlock()
	_, err := os.Stdout.WriteString(out.String())
	return err
}

// placeholders builds the grid of U+10EEEE cells. The foreground colour
// carries the image id; diacritics carry each cell's row and column.
func placeholders(id uint32, cols, rows int) string {
	fg := fmt.Sprintf("\x1b[38;2;%d;%d;%dm", id>>16&0xFF, id>>8&0xFF, id&0xFF)
	var sb strings.Builder
	for r := 0; r < rows; r++ {
		sb.WriteString(fg)
		for c := 0; c < cols; c++ {
			sb.WriteRune(0x10EEEE)
			sb.WriteRune(diacritics[r])
			sb.WriteRune(diacritics[c])
		}
		sb.WriteString("\x1b[39m")
		if r < rows-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// Cleanup frees every image Seshat uploaded. Call it after the TUI exits.
func Cleanup() {
	idMu.Lock()
	defer idMu.Unlock()
	if len(usedIDs) == 0 {
		return
	}
	var out strings.Builder
	for _, id := range usedIDs {
		fmt.Fprintf(&out, "\x1b_Ga=d,d=I,q=2,i=%d\x1b\\", id)
	}
	writeMu.Lock()
	defer writeMu.Unlock()
	os.Stdout.WriteString(out.String())
}
