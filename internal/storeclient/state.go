package storeclient

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// State is the local half of a Store session.
//
// It holds an installation identity and short-lived credentials. It
// never holds a Store secret, a signing private key, or anything the
// local user could use to impersonate the Store.
type State struct {
	InstallationID   string `json:"installation_id"`
	DeviceLabel      string `json:"device_label,omitempty"`
	AccessToken      string `json:"access_token,omitempty"`
	RefreshToken     string `json:"refresh_token,omitempty"`
	AccountPublicID  string `json:"account_public_id,omitempty"`
	Email            string `json:"email,omitempty"`
	BaseURL          string `json:"base_url,omitempty"`
	SigningPublicKey string `json:"signing_public_key,omitempty"`
	KeyFingerprint   string `json:"key_fingerprint,omitempty"`

	dir string `json:"-"`
}

// StateFileName is the on-disk name of the Store session file.
const StateFileName = "store-auth.json"

// LoadState reads an existing session file. A missing file is not an
// error: local TimeKeeper works without a Store account.
func LoadState(dir string) (*State, error) {
	if dir == "" {
		return nil, nil
	}
	path := filepath.Join(dir, StateFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read store state: %w", err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse store state: %w", err)
	}
	state.dir = dir
	state.ensureInstallationID()
	return &state, nil
}

// Save persists the session with owner-only permissions.
func (s *State) Save() error {
	if s == nil || s.dir == "" {
		return nil
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create store state dir: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode store state: %w", err)
	}
	path := filepath.Join(s.dir, StateFileName)
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write store state: %w", err)
	}
	return nil
}

func (s *State) ensureInstallationID() {
	if s == nil {
		return
	}
	if strings.TrimSpace(s.InstallationID) == "" {
		s.InstallationID = NewInstallationID()
	}
}

// NewInstallationID generates a TimeKeeper installation identity.
//
// Deliberately a generated identity rather than a hardware
// fingerprint: it can be revoked and reissued, and it tells the Store
// nothing about the machine.
func NewInstallationID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		// A failed CSPRNG read is fatal for identity generation.
		panic("storeclient: cannot generate installation id: " + err.Error())
	}
	return "TK-DEVICE-" + strings.ToUpper(hex.EncodeToString(buf))
}
