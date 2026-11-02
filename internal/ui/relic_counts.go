package ui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Peejidle/seshat/internal/relics"
)

// Typing in relic counts. With hundreds of relics, one line takes many at
// once ("a s8 12, l n1 4, meso s3 +2") and the box stays open after Enter,
// so a whole inventory goes in without leaving the keyboard's home row.

// relicCountEdit is one parsed entry: set the relic to n, or add n to it.
type relicCountEdit struct {
	relic *relics.Relic
	n     int
	add   bool // n is relative: "+2" or "-1"
}

func (m Model) countKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.relicCounting = false
		m.relicCount.SetValue("")
		m.relicCount.Blur()
		return m, nil
	case "enter":
		edits, err := parseCounts(m.relicCount.Value(), m.relics, m.selectedRelic())
		if err != nil {
			// Nothing is applied, and the text stays so it can be fixed.
			m.relicMsg, m.relicMsgErr = err.Error(), true
			return m, nil
		}
		var done []string
		for _, e := range edits {
			n := e.n
			if e.add {
				n += m.prog.HeldRelics(e.relic.Name)
			}
			m.prog.SetRelics(e.relic.Name, n)
			done = append(done, fmt.Sprintf("%s ×%d", e.relic.Name, m.prog.HeldRelics(e.relic.Name)))
		}
		m.relicMsg, m.relicMsgErr = "✓ "+strings.Join(done, " · "), false
		m.relicCount.SetValue("")
		m.save()
		if len(edits) > 0 {
			m.followRelic(edits[len(edits)-1].relic)
			m.scrollRelics()
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.relicCount, cmd = m.relicCount.Update(k)
	return m, cmd
}

// parseCounts reads comma-separated entries of "tier name count", where
// the tier may be written out or as its first letter and count may be
// "+n" or "-n" to add or remove. A lone count applies to selected.
func parseCounts(s string, t *relics.Table, selected *relics.Relic) ([]relicCountEdit, error) {
	var out []relicCountEdit
	for _, entry := range strings.Split(s, ",") {
		f := strings.Fields(entry)
		var e relicCountEdit
		var count string
		switch len(f) {
		case 0:
			continue
		case 1:
			if selected == nil {
				return nil, fmt.Errorf("%q: no relic selected; write it as tier name count", entry)
			}
			e.relic, count = selected, f[0]
		case 3:
			if e.relic = t.Find(f[0], f[1]); e.relic == nil {
				return nil, fmt.Errorf("no relic called %s %s", f[0], f[1])
			}
			count = f[2]
		default:
			return nil, fmt.Errorf("%q: write it as tier name count, e.g. a s8 12", strings.TrimSpace(entry))
		}
		e.add = strings.HasPrefix(count, "+") || strings.HasPrefix(count, "-")
		n, err := strconv.Atoi(count)
		if err != nil || (!e.add && n < 0) {
			return nil, fmt.Errorf("%q isn't a count", count)
		}
		e.n = n
		out = append(out, e)
	}
	return out, nil
}
