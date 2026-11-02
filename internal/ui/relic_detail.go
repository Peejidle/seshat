package ui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Peejidle/seshat/internal/catalog"
	"github.com/Peejidle/seshat/internal/relics"
)

// The relic window: enter on a relic in the finder opens everything about
// it (its rewards and whether you have them, where it drops) and lets you
// edit the count and check off parts without leaving.

// rewardPart is the item part a relic reward gives.
type rewardPart struct {
	item catalog.Item
	part catalog.Component
}

// matchRewards maps each reward name back to its part, the other way round
// from m.drops. Called from matchDrops.
func (m *Model) matchRewards() {
	m.rewardParts = map[string]rewardPart{}
	for _, it := range m.items {
		for _, c := range it.Parts() {
			for _, d := range m.drops[partKey(it.ID, c.ID)] {
				m.rewardParts[d.Reward.Item] = rewardPart{it, c}
			}
		}
	}
}

func (m *Model) openRelic(r *relics.Relic) {
	m.relicOpen, m.rewardAt, m.relicDigits, m.relicMsg = r, 0, "", ""
}

func (m Model) relicDetailKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	r := m.relicOpen
	key := k.String()
	switch {
	case len(key) == 1 && key[0] >= '0' && key[0] <= '9':
		// Typing a number sets the count once enter is pressed.
		if len(m.relicDigits) < 4 {
			m.relicDigits += key
		}
	case key == "backspace":
		m.relicDigits = m.relicDigits[:max(len(m.relicDigits)-1, 0)]
	case key == "enter" && m.relicDigits != "":
		n, _ := strconv.Atoi(m.relicDigits)
		m.setHeldRelics(r, n)
		m.relicDigits = ""
	case key == "+" || key == "=":
		m.setHeldRelics(r, m.prog.HeldRelics(r.Name)+1)
	case key == "-":
		m.setHeldRelics(r, m.prog.HeldRelics(r.Name)-1)
	case key == "down" || key == "j":
		m.rewardAt = min(m.rewardAt+1, len(r.Rewards)-1)
	case key == "up" || key == "k":
		m.rewardAt = max(m.rewardAt-1, 0)
	case key == "p":
		m.toggleRewardPart(r.Rewards[m.rewardAt])
	case key == "w":
		url := "https://wiki.warframe.com/w/" + strings.ReplaceAll(r.Name, " ", "_") + "_Relic"
		if err := openExternal(url); err != nil {
			m.relicMsg, m.relicMsgErr = fmt.Sprintf("could not open browser: %v", err), true
		}
	case key == "esc" && m.relicDigits != "":
		m.relicDigits = "" // first esc drops a half-typed count
	default:
		// Back to the list, with the cursor still on this relic even if the
		// edits moved it.
		m.relicOpen, m.relicDigits = nil, ""
		m.followRelic(r)
		m.scrollRelics()
	}
	return m, nil
}

func (m *Model) setHeldRelics(r *relics.Relic, n int) {
	m.prog.SetRelics(r.Name, n)
	m.relicMsg, m.relicMsgErr = fmt.Sprintf("✓ %s ×%d", r.Name, m.prog.HeldRelics(r.Name)), false
	m.save()
}

// toggleRewardPart counts one more of a reward's part as held, wrapping to
// none, like p in the item details.
func (m *Model) toggleRewardPart(rw relics.Reward) {
	rp, ok := m.rewardParts[rw.Item]
	if !ok {
		m.relicMsg, m.relicMsgErr = rw.Item+" isn't a part Seshat tracks.", true
		return
	}
	it, c := rp.item, rp.part
	n := (m.prog.HeldParts(it.ID, c.ID) + 1) % (c.Count + 1)
	m.prog.SetPart(it.ID, c.ID, n)
	m.relicMsg, m.relicMsgErr = fmt.Sprintf("✓ %s %s: have %d of %d", it.Name, m.partLabel(it, c), n, c.Count), false
	m.save()
}

// rewardState describes where you stand on a reward: its checkbox, a short
// note and whether you still need it.
func (m Model) rewardState(rw relics.Reward) (box, note string, needed bool) {
	rp, ok := m.rewardParts[rw.Item]
	if !ok {
		return "   ", "", false // Forma and the like
	}
	mk := m.prog.Get(rp.item.ID)
	have := m.prog.HeldParts(rp.item.ID, rp.part.ID)
	switch {
	case mk.Mastered:
		return "[★]", sDim.Render("mastered"), false
	case mk.Owned:
		return "[●]", sDim.Render("built"), false
	case have >= rp.part.Count:
		return "[✓]", sDim.Render(fmt.Sprintf("have %d", have)), false
	case have > 0:
		return "[◐]", sNew.Render(fmt.Sprintf("have %d/%d", have, rp.part.Count)), true
	}
	return "[ ]", sNew.Render("needed"), true
}

func (m Model) relicDetailView() string {
	r := m.relicOpen
	w := max(20, m.width-6)
	var b strings.Builder

	state := sNew.Render("farmable")
	switch {
	case r.Event():
		state = sDim.Render("event relic · no longer drops")
	case !r.Farmable:
		state = sDim.Render("vaulted · trade or Prime Resurgence")
	}
	b.WriteString(sTitle.Render(r.Name+" Relic") + "  " + state + "\n")
	held := sGold.Render(fmt.Sprintf("You hold ×%d", m.prog.HeldRelics(r.Name)))
	if m.relicDigits != "" {
		held = sGold.Render("Set count to "+m.relicDigits+"▌") + sDim.Render("  enter saves · esc cancels")
	}
	b.WriteString(held + "\n\n")

	b.WriteString(sSub.Render("Rewards") + sDim.Render("   chance intact → radiant") + "\n")
	for i, rw := range r.Rewards {
		box, note, needed := m.rewardState(rw)
		sel, st := "  ", sDim
		if needed {
			st = sText
		}
		if i == m.rewardAt {
			sel, st = sGold.Render("▸ "), sTitle
		}
		b.WriteString(sel + box + " " + st.Render(pad(trunc(rw.Item, 36), 37)) + chance(rw) + "  " + note + "\n")
	}

	b.WriteString("\n" + sSub.Render("Drops from") + "\n")
	const shown = 6
	for i, s := range r.Missions {
		if i == shown {
			b.WriteString(sDim.Render(fmt.Sprintf("  …and %d more missions", len(r.Missions)-shown)) + "\n")
			break
		}
		where := fmt.Sprintf("%s, %s · %s", s.Node, s.Planet, s.Mode)
		if s.Rotation != "" {
			where += " · rotation " + s.Rotation
		}
		b.WriteString("  " + sText.Render(pad(trunc(where, 52), 53)) + sDim.Render(fmt.Sprintf("%5.2f%%", s.Chance)) + "\n")
	}
	if len(r.Elsewhere) > 0 {
		b.WriteString(sDim.Render(trunc("  also "+strings.Join(r.Elsewhere, ", "), w)) + "\n")
	}
	if len(r.Missions) == 0 && len(r.Elsewhere) == 0 {
		why := "it's vaulted"
		if r.Event() {
			why = "it was an event reward"
		}
		b.WriteString(sDim.Render("  nowhere right now: "+why) + "\n")
	}

	b.WriteString("\n")
	if m.relicMsg != "" {
		st := sNew
		if m.relicMsgErr {
			st = sErr
		}
		b.WriteString(st.Render(trunc(m.relicMsg, w)) + "\n")
	}
	b.WriteString(sDim.Render(trunc("+/- count · type a number to set it · j/k reward · p have this part", w)) + "\n")
	b.WriteString(sDim.Render(trunc("w wiki · any other key back to the list", w)))
	return b.String()
}
