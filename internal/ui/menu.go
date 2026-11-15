package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// The menu (,): the things done now and then, kept off the key bar along
// the bottom so it only shows the everyday keys. Each still has its own
// key, which works from the main list as before; the menu shows them,
// and picking one is the same as pressing its key there.

// menuItem is one line of the menu: the key it stands for, and what it does.
type menuItem struct {
	key, what string
}

// menuItems lists the menu, some lines saying how things stand now.
func (m Model) menuItems() []menuItem {
	theme := "Seshat's own colours"
	if m.prog.Settings.Theme == "" || m.prog.Settings.Theme == "terminal" {
		theme = "your terminal's colours"
	}
	hide := "Hide mastered items"
	if m.prog.Settings.HideMastered {
		hide = "Show mastered items (hidden now)"
	}
	return []menuItem{
		{"P", "Import from your Warframe profile"},
		{"x", "Ledger: Star Chart, Steel Path and intrinsics XP"},
		{"u", "Sync the item data now"},
		{"H", hide},
		{"T", "Theme: " + theme + " (switch)"},
		{"E", "Export a backup of your records"},
		{"I", "Import records from a backup"},
		{"?", "Help: every key"},
	}
}

func (m *Model) openMenu() {
	m.modal, m.menuAt = mMenu, 0
}

func (m Model) menuKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.menuItems()
	pick := ""
	switch k.String() {
	case "down", "j", "tab":
		m.menuAt = (m.menuAt + 1) % len(items)
		return m, nil
	case "up", "k", "shift+tab":
		m.menuAt = (m.menuAt + len(items) - 1) % len(items)
		return m, nil
	case "enter":
		pick = items[m.menuAt].key
	case "esc", ",", "q":
		m.modal = mNone
		return m, nil
	default:
		for _, it := range items {
			if it.key == k.String() {
				pick = it.key
			}
		}
		if pick == "" {
			return m, nil
		}
	}
	// Close the menu and press the item's key on the main list.
	m.modal = mNone
	return m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(pick)})
}

func (m Model) menuView() string {
	var b strings.Builder
	b.WriteString(sTitle.Render("Menu") + "\n")
	b.WriteString(sSub.Render("Each works from the main list by its key, too.") + "\n\n")
	for i, it := range m.menuItems() {
		sel, st := "  ", sText
		if i == m.menuAt {
			sel, st = sGold.Render("▸ "), sTitle
		}
		b.WriteString(sel + sKey.Render(pad(it.key, 3)) + st.Render(it.what) + "\n")
	}
	b.WriteString("\n" + sDim.Render("j/k choose · enter or its key · esc close"))
	return b.String()
}
