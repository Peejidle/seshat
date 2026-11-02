package ui

import (
	"sort"
	"strings"

	"github.com/Peejidle/seshat/internal/catalog"
)

// The visible list: filtering, sorting, selection and marking.

func (m *Model) rebuild() {
	q := strings.ToLower(strings.TrimSpace(m.search.Value()))
	m.anchor = -1
	m.visible = m.visible[:0]
	for i, it := range m.items {
		if m.tab > 0 && it.Category != catalog.Categories[m.tab-1] {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(it.Name), q) && !strings.Contains(strings.ToLower(it.Type), q) {
			continue
		}
		mk := m.prog.Get(it.ID)
		// The "mastered" filter overrides hiding, or it would always be empty.
		if mk.Mastered && m.prog.Settings.HideMastered && m.filter != fMastered {
			continue
		}
		switch m.filter {
		case fOwnedUnmastered:
			if !mk.Owned || mk.Mastered {
				continue
			}
		case fInProgress:
			if mk.Mastered || mk.Rank == 0 {
				continue
			}
		case fCollecting:
			if held, _ := m.partsProgress(it); mk.Owned || held == 0 {
				continue
			}
		case fMastered:
			if !mk.Mastered {
				continue
			}
		case fNew:
			if !m.newIDs[it.ID] {
				continue
			}
		}
		m.visible = append(m.visible, i)
	}
	items := m.items
	switch m.sort {
	case sType:
		sort.SliceStable(m.visible, func(a, b int) bool {
			ia, ib := items[m.visible[a]], items[m.visible[b]]
			if ia.Category != ib.Category {
				return catIndex(ia.Category) < catIndex(ib.Category)
			}
			return ia.Type < ib.Type
		})
	case sNewest:
		sort.SliceStable(m.visible, func(a, b int) bool {
			return items[m.visible[a]].ReleaseDate > items[m.visible[b]].ReleaseDate
		})
	}
	m.clamp()
}

// rebuildKeep rebuilds but keeps the cursor near the same row, so marking an
// item that then drops out of the filter doesn't jump to the top.
func (m *Model) rebuildKeep() {
	c := m.cursor
	m.rebuild()
	m.cursor = c
	m.clamp()
}

// relist refreshes the list after a mark, except under the details view,
// which must keep showing the item even if it no longer matches the filter.
func (m *Model) relist() {
	if m.modal != mDetail {
		m.rebuildKeep()
	}
}

func catIndex(c catalog.Category) int {
	for i, x := range catalog.Categories {
		if x == c {
			return i
		}
	}
	return len(catalog.Categories)
}

func (m *Model) clamp() {
	if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	h := m.listHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m Model) inSelection(i int) bool {
	if m.anchor < 0 {
		return false
	}
	return i >= min(m.anchor, m.cursor) && i <= max(m.anchor, m.cursor)
}

// targets is what a mark applies to: the bulk selection, or the cursor row.
func (m Model) targets() []catalog.Item {
	if m.anchor < 0 {
		if it, ok := m.current(); ok {
			return []catalog.Item{it}
		}
		return nil
	}
	var out []catalog.Item
	for i := min(m.anchor, m.cursor); i <= max(m.anchor, m.cursor); i++ {
		out = append(out, m.items[m.visible[i]])
	}
	return out
}

// markMastered toggles mastery on the targets. A mixed selection gets
// mastered; only an all-mastered one is cleared. Mastering implies owning.
func (m *Model) markMastered() {
	ts := m.targets()
	if len(ts) == 0 {
		return
	}
	on, xp := false, 0
	for _, it := range ts {
		if !m.prog.Get(it.ID).Mastered {
			on = true
		}
	}
	for _, it := range ts {
		mk := m.prog.Get(it.ID)
		if on && !mk.Mastered {
			xp += it.MasteryXP() - it.EarnedXP(false, mk.Rank)
		}
		mk.Mastered = on
		if on {
			mk.Owned = true
		}
		m.prog.Set(it.ID, mk)
	}
	switch {
	case len(ts) == 1 && on:
		m.say("%s — mastery inscribed. +%s XP", ts[0].Name, commas(xp))
	case len(ts) == 1:
		m.say("%s — mastery struck from the record.", ts[0].Name)
	case on:
		m.say("Mastery inscribed for %d items. +%s XP", len(ts), commas(xp))
	default:
		m.say("Mastery struck from %d items.", len(ts))
	}
	m.save()
	m.relist()
}

// markOwned toggles ownership the same way as markMastered.
func (m *Model) markOwned() {
	ts := m.targets()
	if len(ts) == 0 {
		return
	}
	on := false
	for _, it := range ts {
		if !m.prog.Get(it.ID).Owned {
			on = true
		}
	}
	for _, it := range ts {
		mk := m.prog.Get(it.ID)
		mk.Owned = on
		m.prog.Set(it.ID, mk)
	}
	switch {
	case len(ts) == 1 && on:
		m.say("%s — marked as in your arsenal.", ts[0].Name)
	case len(ts) == 1:
		m.say("%s — marked as not owned.", ts[0].Name)
	case on:
		m.say("%d items marked as in your arsenal.", len(ts))
	default:
		m.say("%d items marked as not owned.", len(ts))
	}
	m.save()
	m.relist()
}

// partKey moves through and checks off the current item's parts: p steps
// the selected part through 0..needed, a fills or clears them all.
func (m *Model) partKey(key string) {
	it, ok := m.current()
	if !ok {
		return
	}
	parts := it.Parts()
	if len(parts) == 0 {
		m.say("%s has no parts to collect.", it.Name)
		return
	}
	m.partAt = min(max(m.partAt, 0), len(parts)-1)
	switch key {
	case "down", "j":
		m.partAt = min(m.partAt+1, len(parts)-1)
		return
	case "up", "k":
		m.partAt = max(m.partAt-1, 0)
		return
	case "p":
		c := parts[m.partAt]
		m.prog.SetPart(it.ID, c.ID, (m.prog.HeldParts(it.ID, c.ID)+1)%(c.Count+1))
	case "a":
		held, need := m.partsProgress(it)
		for _, c := range parts {
			n := 0
			if held < need {
				n = c.Count
			}
			m.prog.SetPart(it.ID, c.ID, n)
		}
	}
	if held, need := m.partsProgress(it); held == need {
		m.say("All parts for %s collected: ready for the foundry.", it.Name)
	} else {
		m.say("%s: %d of %d parts collected.", it.Name, held, need)
	}
	m.save()
}

// partsProgress counts the parts held toward an item and the parts needed.
func (m Model) partsProgress(it catalog.Item) (held, need int) {
	for _, c := range it.Parts() {
		held += min(m.prog.HeldParts(it.ID, c.ID), c.Count)
		need += c.Count
	}
	return held, need
}

func (m Model) current() (catalog.Item, bool) {
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return catalog.Item{}, false
	}
	return m.items[m.visible[m.cursor]], true
}
