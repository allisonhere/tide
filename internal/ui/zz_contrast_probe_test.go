package ui

import "testing"

func TestProbeDimmedContrast(t *testing.T) {
	for _, th := range BuiltinThemes {
		r := contrastRatio(th.Dimmed, th.Bg)
		flag := ""
		if r < 3 {
			flag = "  <-- FAILS 3:1"
		}
		t.Logf("%-28s dimmed=%s bg=%s ratio=%.2f%s", th.Name, th.Dimmed, th.Bg, r, flag)
	}
}
