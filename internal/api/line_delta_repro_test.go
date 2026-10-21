package api

import (
	"strings"
	"testing"
)

// Regression cases for insertions, deletions, reorders, duplicates, and copied lines.
func TestLineDeltaMatchesLongestCommonSubsequenceForSmallInputs(t *testing.T) {
	texts := []string{}
	var build func(prefix []string, remaining int)
	build = func(prefix []string, remaining int) {
		if len(prefix) > 0 {
			texts = append(texts, strings.Join(prefix, "\n"))
		}
		if remaining == 0 {
			return
		}
		for _, line := range []string{"a", "b"} {
			build(append(prefix, line), remaining-1)
		}
	}
	build(nil, 4)
	for _, from := range texts {
		for _, to := range texts {
			fromLines, toLines := strings.Split(from, "\n"), strings.Split(to, "\n")
			lcs := make([][]int, len(fromLines)+1)
			for i := range lcs {
				lcs[i] = make([]int, len(toLines)+1)
			}
			for i := len(fromLines) - 1; i >= 0; i-- {
				for j := len(toLines) - 1; j >= 0; j-- {
					if fromLines[i] == toLines[j] {
						lcs[i][j] = lcs[i+1][j+1] + 1
					} else if lcs[i+1][j] > lcs[i][j+1] {
						lcs[i][j] = lcs[i+1][j]
					} else {
						lcs[i][j] = lcs[i][j+1]
					}
				}
			}
			hunks := lineDelta(from, to)
			added, removed := countHunkChanges(hunks)
			wantAdded := len(toLines) - lcs[0][0]
			wantRemoved := len(fromLines) - lcs[0][0]
			if added != wantAdded || removed != wantRemoved {
				t.Fatalf("lineDelta(%q,%q)=(+%d,-%d), want (+%d,-%d)", from, to, added, removed, wantAdded, wantRemoved)
			}
		}
	}
}

func TestLineDeltaRegressionCases(t *testing.T) {
	cases := []struct {
		name             string
		from, to         string
		wantAdd, wantDel int
	}{
		{"identical", "a\nb\n", "a\nb\n", 0, 0},
		{"pure insert", "a\n", "a\nb\n", 1, 0},
		{"REORDER only", "a\nb\n", "b\na\n", 1, 1},
		{"DUPLICATE only", "a\nb\na\n", "a\nb\na\na\n", 1, 0},
		{"indented copy", "a\n", "a\n  a\n", 1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hunks := lineDelta(tc.from, tc.to)
			add, del := countHunkChanges(hunks)
			if add != tc.wantAdd || del != tc.wantDel {
				t.Errorf("lineDelta(%q, %q) = +%d/-%d, want +%d/-%d",
					tc.from, tc.to, add, del, tc.wantAdd, tc.wantDel)
			}
		})
	}
}

func countHunkChanges(hunks []LineHunk) (int, int) {
	added, removed := 0, 0
	for _, h := range hunks {
		added += h.ToCount
		removed += h.FromCount
	}
	return added, removed
}
