package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Peejidle/seshat/internal/relics"
)

// The recount: a walk through your relics in the order the game lists them
// when sorted by name, so you can go down both lists side by side, typing
// a count only where it changed.

// startRecount lists the relics held, in name order, and starts at the
// first one, or where the last recount was stopped.
func (m *Model) startRecount() {
	m.recountList = nil
	for _, r := range m.relics.Relics {
		if m.prog.HeldRelics(r.Name) > 0 {
			m.recountList = append(m.recountList, r)
		}
	}
	sort.Slice(m.recountList, func(i, j int) bool { return relics.Less(m.recountList[i], m.recountList[j]) })

	m.recountAt = 0
	if m.recountResume != nil {
		m.recountAt = m.recountPlace(m.recountResume)
	}
	m.recountResume = nil
	m.recounting, m.recountDone, m.relicMsg = true, map[*relics.Relic]string{}, ""
	m.relicCount.SetValue("")
	m.relicCount.Focus()
}

// recountPlace is where r is in the recount list, putting it in its place
// by name if it isn't there yet.
func (m *Model) recountPlace(r *relics.Relic) int {
	i := sort.Search(len(m.recountList), func(i int) bool { return !relics.Less(m.recountList[i], r) })
	if i == len(m.recountList) || m.recountList[i] != r {
		m.recountList = append(m.recountList, nil)
		copy(m.recountList[i+1:], m.recountList[i:])
		m.recountList[i] = r
	}
	return i
}

func (m Model) recountKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.stopRecount()
		if m.recountAt < len(m.recountList) {
			m.recountResume = m.recountList[m.recountAt]
			m.relicMsg, m.relicMsgErr = fmt.Sprintf("Recount paused at %s: r carries on from there.", m.recountResume.Name), false
		}
		return m, nil
	case "up":
		m.recountAt = max(m.recountAt-1, 0)
		return m, nil
	case "enter":
		return m.recountEnter()
	}
	var cmd tea.Cmd
	m.relicCount, cmd = m.relicCount.Update(k)
	return m, cmd
}

// recountEnter handles one line of the recount: empty keeps the current
// relic's count, a number sets it, and "tier name count" sets any relic and
// carries on after it.
func (m Model) recountEnter() (tea.Model, tea.Cmd) {
	var cur *relics.Relic
	if m.recountAt < len(m.recountList) {
		cur = m.recountList[m.recountAt]
	}
	in := strings.TrimSpace(m.relicCount.Value())
	if in == "" {
		if cur != nil {
			m.recountDone[cur] = "kept"
			m.recountAt++
		}
		return m.recountNext()
	}

	edits, err := parseCounts(in, m.relics, cur)
	if err != nil {
		m.relicMsg, m.relicMsgErr = err.Error(), true
		return m, nil
	}
	next := m.recountAt
	for _, e := range edits {
		n := e.n
		if e.add {
			n += m.prog.HeldRelics(e.relic.Name)
		}
		m.prog.SetRelics(e.relic.Name, n)
		m.recountDone[e.relic] = fmt.Sprintf("now %d", m.prog.HeldRelics(e.relic.Name))
		if m.prog.HeldRelics(e.relic.Name) == 0 {
			m.recountDone[e.relic] = "removed"
		}
		next = m.recountPlace(e.relic) + 1
	}
	m.save()
	m.recountAt = next
	m.relicMsg = ""
	m.relicCount.SetValue("")
	return m.recountNext()
}

// recountNext finishes the recount once it has gone past the last relic.
func (m Model) recountNext() (tea.Model, tea.Cmd) {
	if m.recountAt < len(m.recountList) {
		return m, nil
	}
	m.stopRecount()
	m.prog.RelicsCounted = time.Now()
	m.save()
	m.relicMine, m.relicAt, m.relicTop = true, 0, 0
	m.relicMsg, m.relicMsgErr = fmt.Sprintf("✓ Recount done: %d relics held in all.", m.heldRelics()), false
	return m, nil
}

func (m *Model) stopRecount() {
	m.recounting = false
	m.relicCount.SetValue("")
	m.relicCount.Blur()
}

func (m Model) recountView() string {
	w := max(20, m.width-6)
	var b strings.Builder
	b.WriteString(sTitle.Render("Recount") + sDim.Render(fmt.Sprintf("   %d of %d", min(m.recountAt+1, len(m.recountList)), len(m.recountList))) + "\n")
	b.WriteString(sSub.Render(trunc("Go down the game's relic list, sorted by name, alongside this one.", w)) + "\n\n")

	// A window of the list around the current relic: a few done above, the
	// rest of the screen for what's coming, to match against the game.
	// The finder's count box, dressed for the recount. m is a copy, so
	// this only changes how it's drawn here.
	m.relicCount.Prompt, m.relicCount.Placeholder = "", "enter: same"
	m.relicCount.Width = max(10, w-24)

	rows := max(4, m.height-14)
	top := max(0, min(m.recountAt-3, len(m.recountList)-rows))
	for i := top; i < len(m.recountList) && i < top+rows; i++ {
		r := m.recountList[i]
		name := pad(r.Name, 13)
		held := pad(fmt.Sprintf("×%d", m.prog.HeldRelics(r.Name)), 6)
		switch {
		case i == m.recountAt:
			b.WriteString(sGold.Render("▸ ") + sTitle.Render(name) + sGold.Render(held) + m.relicCount.View() + "\n")
		case i < m.recountAt:
			b.WriteString("  " + sDim.Render(name+held) + sNew.Render("✓ "+m.recountDone[r]) + "\n")
		default:
			b.WriteString("  " + sText.Render(name) + sDim.Render(held) + "\n")
		}
	}
	if len(m.recountList) == 0 {
		b.WriteString(sDim.Render("No relics counted yet. Type each as you go: l a1 3") + "\n")
	}
	if m.recountAt >= len(m.recountList) && len(m.recountList) > 0 {
		b.WriteString(sGold.Render("▸ ") + m.relicCount.View() + "\n")
	}

	b.WriteString("\n")
	if m.relicMsg != "" {
		b.WriteString(sErr.Render(trunc(m.relicMsg, w)) + "\n")
	}
	b.WriteString(sDim.Render(trunc("enter keeps the count · a number sets it · 0 removes · ↑ back", w)) + "\n")
	b.WriteString(sDim.Render(trunc("one the list doesn't have? type it in full: l b4 3 · esc pauses", w)))
	return b.String()
}

// countedAgo says how long ago the relics were last recounted.
func (m Model) countedAgo() string {
	t := m.prog.RelicsCounted
	if t.IsZero() {
		return "never recounted"
	}
	switch days := int(time.Since(t).Hours() / 24); days {
	case 0:
		return "recounted today"
	case 1:
		return "recounted yesterday"
	default:
		return fmt.Sprintf("recounted %d days ago", days)
	}
}
