package termimg

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// The TUI lays out placeholders with lipgloss, so each placeholder cell must
// measure exactly one column wide or panes will misalign.
func TestPlaceholderWidth(t *testing.T) {
	out := placeholders(0xABCDEF, 40, 20)
	lines := strings.Split(out, "\n")
	if len(lines) != 20 {
		t.Fatalf("rows = %d, want 20", len(lines))
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w != 40 {
			t.Fatalf("row %d width = %d, want 40", i, w)
		}
	}
	if !strings.HasPrefix(lines[0], "\x1b[38;2;171;205;239m") {
		t.Errorf("image id not encoded in foreground: %q", lines[0][:24])
	}
	if got := lipgloss.Width(lipgloss.PlaceHorizontal(50, lipgloss.Center, out)); got != 50 {
		t.Errorf("placed width = %d, want 50", got)
	}
}
