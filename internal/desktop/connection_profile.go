package desktop

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
)

type ConnectionProfileMode string

const (
	ConnectionProfileLocal   ConnectionProfileMode = "local"
	ConnectionProfileRemote  ConnectionProfileMode = "remote"
	ConnectionAuthNone       string                = "none"
	ConnectionAuthAPIKey     string                = "api_key"
	maxConnectionProfileSize                       = 32 << 10
)

// ConnectionProfile describes an explicit client-side connection boundary.
// CredentialRef is only an opaque OS-secret-store reference; raw credentials
// are intentionally not represented in this type.
type ConnectionProfile struct {
	ID                 string                `json:"id"`
	Name               string                `json:"name"`
	Mode               ConnectionProfileMode `json:"mode"`
	PrimaryEndpoint    string                `json:"primary_endpoint"`
	SecondaryEndpoint  string                `json:"secondary_endpoint,omitempty"`
	AutomaticFailover  bool                  `json:"automatic_failover"`
	ExpectedInstanceID string                `json:"expected_instance_id,omitempty"`
	AuthenticationMode string                `json:"authentication_mode"`
	CredentialRef      string                `json:"credential_ref,omitempty"`
}

// DefaultLocalConnectionProfile is the fixed local contact policy used by the
// desktop: friendly hostname first, numeric loopback only as its fallback.
func DefaultLocalConnectionProfile() ConnectionProfile {
	return ConnectionProfile{
		ID:                 "local-default",
		Name:               "Local TimeKeeper",
		Mode:               ConnectionProfileLocal,
		PrimaryEndpoint:    DefaultLocalPrimaryURL,
		SecondaryEndpoint:  DefaultLocalFallbackURL,
		AuthenticationMode: ConnectionAuthNone,
	}
}

func localConnectionProfileFromConfig(cfg Config) (ConnectionProfile, error) {
	profile := DefaultLocalConnectionProfile()
	primary := strings.TrimSpace(cfg.CoreURL)
	if primary == "" {
		return ConnectionProfile{}, fmt.Errorf("local Core primary endpoint is required")
	}
	profile.PrimaryEndpoint = primary
	if isFriendlyLocalURL(primary) {
		if fallback := strings.TrimSpace(cfg.FallbackURL); fallback != "" {
			profile.SecondaryEndpoint = fallback
		}
	} else {
		profile.SecondaryEndpoint = ""
	}
	if err := profile.Validate(); err != nil {
		return ConnectionProfile{}, fmt.Errorf("%w: invalid local connection profile: %v", ErrNotConfigured, err)
	}
	return profile, nil
}

func (p ConnectionProfile) Validate() error {
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("connection profile ID and name are required")
	}
	if p.ID != strings.TrimSpace(p.ID) || p.Name != strings.TrimSpace(p.Name) {
		return fmt.Errorf("connection profile ID and name must not have surrounding whitespace")
	}
	if p.ExpectedInstanceID != "" && !validConnectionInstanceID(p.ExpectedInstanceID) {
		return fmt.Errorf("connection profile expected instance ID is invalid")
	}

	switch p.Mode {
	case ConnectionProfileLocal:
		if p.AuthenticationMode != "" && p.AuthenticationMode != ConnectionAuthNone {
			return fmt.Errorf("local connection profiles do not support remote authentication")
		}
		if p.CredentialRef != "" || p.AutomaticFailover {
			return fmt.Errorf("local connection profiles cannot configure credentials or remote failover")
		}
		if !validLocalProfileEndpoint(p.PrimaryEndpoint) {
			return fmt.Errorf("local primary endpoint must be timekeeper.local or a numeric loopback origin")
		}
		if isFriendlyLocalURL(p.PrimaryEndpoint) {
			if !isNumericLoopbackURL(p.SecondaryEndpoint) {
				return fmt.Errorf("friendly local profiles require a numeric-loopback fallback")
			}
		} else if p.SecondaryEndpoint != "" {
			return fmt.Errorf("numeric-loopback local profiles must not configure a second fallback")
		}
	case ConnectionProfileRemote:
		if !validRemoteProfileEndpoint(p.PrimaryEndpoint) {
			return fmt.Errorf("remote primary endpoint must be an HTTPS origin or versioned API base")
		}
		if p.SecondaryEndpoint != "" && !validRemoteProfileEndpoint(p.SecondaryEndpoint) {
			return fmt.Errorf("remote secondary endpoint must be an HTTPS origin or versioned API base")
		}
		if p.AuthenticationMode != ConnectionAuthAPIKey {
			return fmt.Errorf("remote profiles require API-key authentication")
		}
		if !validCredentialReference(p.CredentialRef) {
			return fmt.Errorf("remote profile requires an OS credential reference; raw API keys are not accepted")
		}
		if p.AutomaticFailover {
			if p.SecondaryEndpoint == "" {
				return fmt.Errorf("automatic failover requires a secondary endpoint")
			}
			if !validConnectionInstanceID(p.ExpectedInstanceID) {
				return fmt.Errorf("automatic failover requires a valid expected instance ID")
			}
		}
	default:
		return fmt.Errorf("connection profile mode must be local or remote")
	}
	return nil
}

// CandidateEndpoints returns configured endpoints in probe order. Remote mode
// never adds either local endpoint; automatic failover remains gated by
// ValidateFailoverPair before switching to a remote secondary.
func (p ConnectionProfile) CandidateEndpoints() ([]string, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if p.Mode == ConnectionProfileLocal {
		endpoints := []string{p.PrimaryEndpoint}
		if p.SecondaryEndpoint != "" {
			endpoints = append(endpoints, p.SecondaryEndpoint)
		}
		return endpoints, nil
	}
	endpoints := []string{p.PrimaryEndpoint}
	if p.AutomaticFailover {
		endpoints = append(endpoints, p.SecondaryEndpoint)
	}
	return endpoints, nil
}

// ValidateFailoverPair proves that both remote endpoints match the configured
// logical instance and the minimum Core contract before automatic switching.
func (p ConnectionProfile) ValidateFailoverPair(primary, secondary ConnectionInfo) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.Mode != ConnectionProfileRemote || !p.AutomaticFailover {
		return fmt.Errorf("profile does not enable remote automatic failover")
	}
	for _, endpoint := range []struct {
		label string
		info  ConnectionInfo
	}{{"primary", primary}, {"secondary", secondary}} {
		if endpoint.info.Service != "TimeKeeper" {
			return fmt.Errorf("%s endpoint is not a TimeKeeper Core", endpoint.label)
		}
		if endpoint.info.InstanceID != p.ExpectedInstanceID {
			return fmt.Errorf("%s endpoint instance ID does not match the expected instance", endpoint.label)
		}
		if endpoint.info.APIVersion != "v1" {
			return fmt.Errorf("%s endpoint API version %q is incompatible", endpoint.label, endpoint.info.APIVersion)
		}
		for _, required := range []string{"projects.list", "tasks.lifecycle"} {
			if !hasConnectionCapability(endpoint.info.Capabilities, required) {
				return fmt.Errorf("%s endpoint lacks required capability %q", endpoint.label, required)
			}
		}
	}
	if primary.InstanceID != secondary.InstanceID {
		return fmt.Errorf("primary and secondary instance IDs differ")
	}
	return nil
}

// DecodeConnectionProfile parses one bounded JSON object, rejects unknown
// fields (including accidental raw-key fields), and validates the profile.
func DecodeConnectionProfile(payload []byte) (ConnectionProfile, error) {
	if len(payload) == 0 || len(payload) > maxConnectionProfileSize {
		return ConnectionProfile{}, fmt.Errorf("connection profile JSON must be between 1 and %d bytes", maxConnectionProfileSize)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var profile ConnectionProfile
	if err := decoder.Decode(&profile); err != nil {
		return ConnectionProfile{}, fmt.Errorf("decode connection profile: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return ConnectionProfile{}, fmt.Errorf("connection profile must contain exactly one JSON object")
		}
		return ConnectionProfile{}, fmt.Errorf("decode trailing connection profile data: %w", err)
	}
	if err := profile.Validate(); err != nil {
		return ConnectionProfile{}, err
	}
	return profile, nil
}

func validLocalProfileEndpoint(raw string) bool {
	if isFriendlyLocalURL(raw) {
		_, _, err := parseCoreURL(raw)
		return err == nil
	}
	return isNumericLoopbackURL(raw)
}

func validRemoteProfileEndpoint(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || host == "timekeeper.local" || host == "api.timekeeper.local" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return false
	}
	return true
}

func validCredentialReference(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (u.Scheme == "keychain" || u.Scheme == "credential") && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}
