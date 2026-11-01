package mastery

import "testing"

func TestRank(t *testing.T) {
	cases := []struct {
		xp   int
		want int
	}{
		{0, 0}, {2499, 0}, {2500, 1}, {9999, 1}, {10000, 2},
		{2_249_999, 29}, {2_250_000, 30}, {2_397_500, 31}, {2_545_000, 32},
	}
	for _, c := range cases {
		if got, _, _ := Rank(c.xp); got != c.want {
			t.Errorf("Rank(%d) = %d, want %d", c.xp, got, c.want)
		}
	}
	if Label(31) != "Legendary 1" || Label(24) != "MR 24" {
		t.Errorf("labels: %q %q", Label(31), Label(24))
	}
}
