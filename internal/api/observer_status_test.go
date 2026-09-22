// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package api

import (
	"encoding/json"
	"testing"
)

// TestObserverStatusSerialised proves the observer state survives JSON
// encoding with the documented key names.
//
// This matters because an agent reads the status payload, not the Go
// struct: if the keys drifted, the capability would silently read as
// absent forever.
func TestObserverStatusSerialised(t *testing.T) {
	cases := []struct {
		name      string
		status    RuntimeStatus
		wantEnab  bool
		wantAvail bool
	}{
		{"absent", RuntimeStatus{}, false, false},
		{"enabled and usable", RuntimeStatus{ObserverEnabled: true, ObserverAvailable: true}, true, true},
		{"enabled but broken", RuntimeStatus{ObserverEnabled: true, ObserverAvailable: false}, true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.status)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}

			var decoded map[string]any
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			// The keys are part of the contract. Absence of a key is
			// not the same as a false value, so assert presence too.
			gotEnab, ok := decoded["observer_enabled"]
			if !ok {
				t.Fatalf("observer_enabled key missing from %s", raw)
			}
			if gotEnab != tc.wantEnab {
				t.Fatalf("observer_enabled = %v, want %v", gotEnab, tc.wantEnab)
			}

			gotAvail, ok := decoded["observer_available"]
			if !ok {
				t.Fatalf("observer_available key missing from %s", raw)
			}
			if gotAvail != tc.wantAvail {
				t.Fatalf("observer_available = %v, want %v", gotAvail, tc.wantAvail)
			}
		})
	}
}
