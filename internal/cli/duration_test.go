package cli

import (
	"testing"
	"time"
)

func TestParseDurationAcceptsUnitsAndBareSeconds(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  time.Duration
	}{
		{name: "minutes", input: "30m", want: 30 * time.Minute},
		{name: "compound", input: "1h30m", want: 90 * time.Minute},
		{name: "bare seconds", input: "3600", want: time.Hour},
		{name: "trimmed bare seconds", input: " 900 ", want: 15 * time.Minute},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseDuration(test.input)
			if err != nil {
				t.Fatalf("parseDuration(%q): %v", test.input, err)
			}
			if got != test.want {
				t.Fatalf("parseDuration(%q) = %s, want %s", test.input, got, test.want)
			}
		})
	}
}

func TestParseDurationRejectsNonPositiveInvalidAndOverflowValues(t *testing.T) {
	for _, input := range []string{"", "0", "0s", "-1", "-1m", "not-a-duration", "9223372037"} {
		t.Run(input, func(t *testing.T) {
			if got, err := parseDuration(input); err == nil || got != 0 {
				t.Fatalf("parseDuration(%q) = %s, %v; want an error and zero", input, got, err)
			}
		})
	}
}

func TestParseDurationStrUsesSharedDurationSemantics(t *testing.T) {
	if got := parseDurationStr("3600"); got != 3600 {
		t.Fatalf("parseDurationStr(3600) = %d, want 3600 seconds", got)
	}
	if got := parseDurationStr("1h30m"); got != 5400 {
		t.Fatalf("parseDurationStr(1h30m) = %d, want 5400 seconds", got)
	}
}
