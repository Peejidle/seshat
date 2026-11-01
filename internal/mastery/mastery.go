// Package mastery converts mastery XP into Mastery Rank.
package mastery

import "strconv"

// Threshold is the total XP needed to reach rank r. Ranks 1-30 follow
// 2500*r^2; Legendary ranks (31+) each add a flat 147,500.
func Threshold(r int) int {
	if r <= 30 {
		return 2500 * r * r
	}
	return 2_250_000 + 147_500*(r-30)
}

// Rank returns the rank reached with xp, plus the XP floor of that rank
// and of the next one.
func Rank(xp int) (rank, cur, next int) {
	for Threshold(rank+1) <= xp {
		rank++
	}
	return rank, Threshold(rank), Threshold(rank + 1)
}

// Label renders a rank as the game shows it, e.g. "MR 24" or "Legendary 2".
func Label(rank int) string {
	if rank > 30 {
		return "Legendary " + strconv.Itoa(rank-30)
	}
	return "MR " + strconv.Itoa(rank)
}
