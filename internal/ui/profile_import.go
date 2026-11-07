package ui

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Peejidle/seshat/internal/profile"
)

// Importing from the Warframe profile: P opens the player's public profile
// in their own browser, they copy it there, and Seshat reads the clipboard.
// The browser makes the one request, never Seshat: DE rate-limits that
// endpoint and hasn't sanctioned third-party use.

type profStep int

const (
	profAskID profStep = iota // no account ID yet
	profCopy                  // the profile is open in the browser: waiting for it to be copied
	profFile                  // reading a saved file instead
	profDone                  // showing what the import did
)

// reopenAfter keeps P from fetching the profile again when it was opened
// moments ago, e.g. after esc and P: repeat requests risk a block.
const reopenAfter = 2 * time.Minute

// clipboardMsg carries the clipboard's text, read in the background.
type clipboardMsg struct {
	data []byte
	err  error
}

// playerIDRe finds an account ID: 24 hex digits, wherever they are in what
// was pasted (the bare ID, the cookie's text, or the whole profile URL).
var playerIDRe = regexp.MustCompile(`[0-9a-fA-F]{24}`)

func (m *Model) openProfileImport() tea.Cmd {
	m.modal, m.profErr, m.profLines = mProfile, "", nil
	if m.prog.Settings.ProfileID == "" {
		m.profStep = profAskID
		m.profID.SetValue("")
		return m.profID.Focus()
	}
	m.profStep = profCopy
	if time.Since(m.profOpened) < reopenAfter {
		return nil // still open from a moment ago; o opens it again
	}
	return m.openProfilePage()
}

// openProfilePage has the browser load the profile, which it then shows as
// JSON, ready to copy.
func (m *Model) openProfilePage() tea.Cmd {
	m.profOpened = time.Now()
	url := profile.URL + m.prog.Settings.ProfileID
	return func() tea.Msg {
		if err := openExternal(url); err != nil {
			return clipboardMsg{err: errors.New("could not open the browser: " + err.Error())}
		}
		return nil
	}
}

func (m Model) profileKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if k.String() == "esc" {
		m.profID.Blur()
		m.pathIn.Blur()
		m.modal = mNone
		return m, nil
	}
	switch m.profStep {
	case profAskID:
		if k.String() != "enter" {
			var cmd tea.Cmd
			m.profID, cmd = m.profID.Update(k)
			return m, cmd
		}
		id := strings.ToLower(playerIDRe.FindString(m.profID.Value()))
		if id == "" {
			m.profErr = "That isn't an account ID: it's 24 letters and digits, 0-9 and a-f."
			return m, nil
		}
		m.prog.Settings.ProfileID, m.profErr = id, ""
		m.save()
		m.profID.Blur()
		m.profStep = profCopy
		return m, m.openProfilePage()

	case profCopy:
		if k.Paste {
			// Pasted into the terminal (ctrl+shift+v), for systems where
			// Seshat can't read the clipboard itself.
			m.importProfileData([]byte(string(k.Runes)))
			return m, nil
		}
		switch k.String() {
		case "enter":
			m.profErr = ""
			return m, readClipboard
		case "o":
			return m, m.openProfilePage()
		case "i":
			m.profStep = profAskID
			m.profID.SetValue(m.prog.Settings.ProfileID)
			m.profID.CursorEnd()
			return m, m.profID.Focus()
		case "f":
			m.profStep, m.profErr = profFile, ""
			m.pathIn.SetValue(guessProfilePath())
			m.pathIn.CursorEnd()
			return m, m.pathIn.Focus()
		}

	case profFile:
		if k.String() != "enter" {
			var cmd tea.Cmd
			m.pathIn, cmd = m.pathIn.Update(k)
			return m, cmd
		}
		b, err := os.ReadFile(expandHome(strings.TrimSpace(m.pathIn.Value())))
		if err != nil {
			m.profErr = err.Error()
			return m, nil
		}
		m.pathIn.Blur()
		m.importProfileData(b)

	case profDone:
		m.modal = mNone // any key closes the report
	}
	return m, nil
}

// importProfileData reads a profile, applies it and shows what it did. On a
// bad read it stays on the same step, saying what went wrong.
func (m *Model) importProfileData(b []byte) {
	p, err := profile.Parse(b)
	if err != nil {
		m.profErr = "That isn't the profile: in the browser, open Raw Data and press Copy (or select all and copy), then enter here."
		if m.profStep == profFile {
			m.profErr = err.Error()
		}
		return
	}
	r := profile.Apply(p, m.items, m.prog, m.nodeXP)
	m.save()
	m.profStep, m.profLines, m.profErr = profDone, profile.Summary(r), ""
	m.say("✓ Profile of %s imported.", r.Name)
	m.rebuildKeep()
}

// readClipboard asks the desktop for the clipboard's text, through whichever
// tool the system has: Go has no clipboard of its own.
func readClipboard() tea.Msg {
	for _, c := range clipboardCommands() {
		if _, err := exec.LookPath(c[0]); err != nil {
			continue
		}
		out, err := exec.Command(c[0], c[1:]...).Output()
		return clipboardMsg{out, err}
	}
	return clipboardMsg{err: errors.New("no clipboard tool found (install wl-clipboard or xclip): paste with ctrl+shift+v instead")}
}

func clipboardCommands() [][]string {
	switch runtime.GOOS {
	case "darwin":
		return [][]string{{"pbpaste"}}
	case "windows":
		return [][]string{{"powershell", "-NoProfile", "-Command", "Get-Clipboard -Raw"}}
	}
	cmds := [][]string{{"xclip", "-selection", "clipboard", "-o"}, {"xsel", "--clipboard", "--output"}}
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		cmds = append([][]string{{"wl-paste", "--no-newline"}}, cmds...)
	}
	return cmds
}

// guessProfilePath suggests the newest profile the browser saved to
// ~/Downloads, which browsers name after the endpoint.
func guessProfilePath() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, "Downloads")
	entries, _ := os.ReadDir(dir)
	var best string
	var newest time.Time
	for _, e := range entries {
		name := strings.ToLower(e.Name())
		if !strings.Contains(name, "profileviewingdata") && !strings.Contains(name, "profiledata") {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().After(newest) {
			best, newest = e.Name(), info.ModTime()
		}
	}
	return "~/Downloads/" + best
}

func (m Model) profileView() string {
	w := max(20, m.width-10)
	var b strings.Builder
	b.WriteString(sTitle.Render("Import from your Warframe profile") + "\n")
	b.WriteString(sSub.Render(trunc("Your browser fetches it, never Seshat. Fills in mastery, intrinsics and nodes.", w)) + "\n\n")

	switch m.profStep {
	case profAskID:
		b.WriteString(sText.Render("Your account ID, asked for once:") + "\n")
		b.WriteString(sDim.Render(trunc("log in at warframe.com, press F12 → Storage (Chrome: Application)", w)) + "\n")
		b.WriteString(sDim.Render(trunc("→ Cookies → user-info, and copy the 24-character user_id.", w)) + "\n\n")
		b.WriteString(m.profID.View() + "\n")
	case profCopy:
		b.WriteString(sText.Render("Your profile is open in the browser. There:") + "\n")
		b.WriteString(sGold.Render("  1 ") + sText.Render("Firefox: Raw Data → Copy.  Others: select all and copy.") + "\n")
		b.WriteString(sGold.Render("  2 ") + sText.Render("Back here, press enter.") + "\n\n")
		b.WriteString(sDim.Render(trunc("One load per import: repeat requests can briefly block you from the game.", w)) + "\n")
	case profFile:
		b.WriteString(sText.Render("A profile saved from the browser:") + "\n")
		b.WriteString(m.pathIn.View() + "\n")
	case profDone:
		for _, l := range m.profLines {
			st := sText
			if strings.Contains(l, "isn't marked right") || strings.Contains(l, "left as they were") {
				st = sGold
			}
			b.WriteString(st.Width(w).Render(l) + "\n")
		}
	}

	if m.profErr != "" {
		b.WriteString("\n" + sErr.Width(w).Render(m.profErr) + "\n")
	}
	b.WriteString("\n")
	switch m.profStep {
	case profAskID, profFile:
		b.WriteString(sDim.Render("enter go on · esc cancel"))
	case profCopy:
		b.WriteString(sDim.Render(trunc("enter read the clipboard · o open it again · f from a file · i change ID · esc", w)))
	case profDone:
		b.WriteString(sDim.Render("any key to close"))
	}
	return b.String()
}
