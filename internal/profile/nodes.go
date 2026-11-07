package profile

import (
	"bytes"
	"os"
	"regexp"
	"strconv"
)

// The wiki's star chart (Module:Missions/data) is Lua source, one node to a
// line, e.g.
//
//	{ Name = "Tolstoj", ..., MasteryExp = 25, InternalName = "SolNode108", ... },
//
// Only those two fields matter here, so each line is searched for them
// rather than parsing the Lua.
var (
	masteryExpRe   = regexp.MustCompile(`\bMasteryExp\s*=\s*(\d+)`)
	internalNameRe = regexp.MustCompile(`\bInternalName\s*=\s*"([^"]+)"`)
)

// LoadNodeXP reads the cached wiki star chart into node name -> mastery XP.
func LoadNodeXP(path string) (map[string]int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseNodeXP(b), nil
}

// ParseNodeXP picks the mastery XP of every named node out of the wiki's
// star chart. Nodes worth nothing (relays, hubs) are kept too, at 0.
func ParseNodeXP(b []byte) map[string]int {
	nodes := map[string]int{}
	for _, line := range bytes.Split(b, []byte("\n")) {
		xp, name := masteryExpRe.FindSubmatch(line), internalNameRe.FindSubmatch(line)
		if xp == nil || name == nil {
			continue
		}
		n, _ := strconv.Atoi(string(xp[1]))
		nodes[string(name[1])] = n
	}
	return nodes
}
