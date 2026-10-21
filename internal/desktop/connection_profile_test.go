package desktop

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestDefaultLocalConnectionProfileUsesFriendlyPrimaryAndLoopbackFallback(t *testing.T) {
	profile := DefaultLocalConnectionProfile()
	if err := profile.Validate(); err != nil {
		t.Fatal(err)
	}
	if profile.Mode != ConnectionProfileLocal || profile.PrimaryEndpoint != DefaultLocalPrimaryURL || profile.SecondaryEndpoint != DefaultLocalFallbackURL {
		t.Fatalf("local profile=%+v", profile)
	}
	endpoints, err := profile.CandidateEndpoints()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(endpoints, []string{DefaultLocalPrimaryURL, DefaultLocalFallbackURL}) {
		t.Fatalf("local candidates=%v", endpoints)
	}
}

func TestRemoteConnectionProfileNeverCandidatesLocalFallback(t *testing.T) {
	profile := ConnectionProfile{
		ID:                 "remote-production",
		Name:               "Production",
		Mode:               ConnectionProfileRemote,
		PrimaryEndpoint:    "https://tk.example.com/v1",
		SecondaryEndpoint:  "https://tk-alt.example.com/v1",
		AutomaticFailover:  true,
		ExpectedInstanceID: "tk_instance_0123456789abcdef0123456789abcdef",
		AuthenticationMode: "api_key",
		CredentialRef:      "keychain://timekeeper/production-api-key",
	}
	endpoints, err := profile.CandidateEndpoints()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(endpoints, []string{profile.PrimaryEndpoint, profile.SecondaryEndpoint}) {
		t.Fatalf("remote candidates=%v", endpoints)
	}
	for _, endpoint := range endpoints {
		if endpoint == DefaultLocalFallbackURL || endpoint == DefaultLocalPrimaryURL {
			t.Fatalf("remote profile leaked into a local endpoint: %q", endpoint)
		}
	}
}

func TestRemoteFailoverPairRequiresExpectedSameInstanceAndCoreCapabilities(t *testing.T) {
	profile := ConnectionProfile{
		ID:                 "remote-production",
		Name:               "Production",
		Mode:               ConnectionProfileRemote,
		PrimaryEndpoint:    "https://tk.example.com/v1",
		SecondaryEndpoint:  "https://tk-alt.example.com/v1",
		AutomaticFailover:  true,
		ExpectedInstanceID: "tk_instance_0123456789abcdef0123456789abcdef",
		AuthenticationMode: "api_key",
		CredentialRef:      "keychain://timekeeper/production-api-key",
	}
	primary := ConnectionInfo{
		Service: "TimeKeeper", InstanceID: profile.ExpectedInstanceID, Version: "1.2.0", APIVersion: "v1",
		Capabilities: []string{"projects.list", "tasks.lifecycle"},
	}
	secondary := primary
	if err := profile.ValidateFailoverPair(primary, secondary); err != nil {
		t.Fatalf("matching failover pair rejected: %v", err)
	}

	secondary.InstanceID = "tk_instance_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := profile.ValidateFailoverPair(primary, secondary); err == nil {
		t.Fatal("different Core instance IDs must reject automatic failover")
	}
	secondary = primary
	secondary.APIVersion = "v2"
	if err := profile.ValidateFailoverPair(primary, secondary); err == nil {
		t.Fatal("different API contracts must reject automatic failover")
	}
	secondary = primary
	secondary.Capabilities = []string{"projects.list"}
	if err := profile.ValidateFailoverPair(primary, secondary); err == nil {
		t.Fatal("missing required capability must reject automatic failover")
	}
}

func TestDecodeConnectionProfileRejectsRawKeyFieldsAndTrailingJSON(t *testing.T) {
	base := map[string]any{
		"id": "remote-production", "name": "Production", "mode": "remote",
		"primary_endpoint": "https://tk.example.com/v1", "authentication_mode": "api_key",
		"credential_ref": "keychain://timekeeper/production-api-key",
	}
	withRawKey := make(map[string]any, len(base)+1)
	for key, value := range base {
		withRawKey[key] = value
	}
	withRawKey["api_key"] = "must-never-be-stored"
	payload, err := json.Marshal(withRawKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeConnectionProfile(payload); err == nil {
		t.Fatal("raw API-key fields must be rejected as unknown profile data")
	}

	validPayload, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	withTrailing := append(append([]byte(nil), validPayload...), []byte(` {"id":"second"}`)...)
	if _, err := DecodeConnectionProfile(withTrailing); err == nil {
		t.Fatal("trailing JSON must be rejected")
	}
}
