package desktop

import (
	"context"
	"errors"
	"fmt"
)

// ConnectionDiagnostic is a read-only report of the endpoint selected by the
// local profile and the Core compatibility metadata returned by that endpoint.
type ConnectionDiagnostic struct {
	ClientSurface    string         `json:"client_surface"`
	SelectedEndpoint string         `json:"selected_endpoint"`
	UsedFallback     bool           `json:"used_fallback"`
	ConnectionInfo   ConnectionInfo `json:"connection_info"`
}

type localCoreSelection struct {
	client       *CoreClient
	addr         string
	usedFallback bool
	probeErr     error
	launchConfig Config
}

// selectLocalCore applies one deterministic local endpoint policy for startup
// and diagnostics. It never uses the local fallback for a non-friendly URL.
func selectLocalCore(ctx context.Context, cfg Config) (localCoreSelection, error) {
	profile, err := localConnectionProfileFromConfig(cfg)
	if err != nil {
		return localCoreSelection{}, err
	}
	endpoints, err := profile.CandidateEndpoints()
	if err != nil {
		return localCoreSelection{}, err
	}
	primaryURL := endpoints[0]
	client, err := NewCoreClient(primaryURL)
	if err != nil {
		return localCoreSelection{}, err
	}
	_, addr, err := parseCoreURL(primaryURL)
	if err != nil {
		return localCoreSelection{}, err
	}
	selection := localCoreSelection{
		client:       client,
		addr:         addr,
		probeErr:     client.probe(ctx),
		launchConfig: cfg,
	}
	if selection.probeErr == nil || ctx.Err() != nil || !errors.Is(selection.probeErr, ErrCoreUnavailable) || len(endpoints) < 2 {
		return selection, nil
	}

	fallbackURL := endpoints[1]
	fallbackClient, err := NewCoreClient(fallbackURL)
	if err != nil {
		return localCoreSelection{}, fmt.Errorf("%w: invalid local fallback URL: %v", ErrNotConfigured, err)
	}
	_, fallbackAddr, err := parseCoreURL(fallbackURL)
	if err != nil {
		return localCoreSelection{}, fmt.Errorf("%w: invalid local fallback URL: %v", ErrNotConfigured, err)
	}
	fallbackConfig := cfg
	fallbackConfig.CoreURL = fallbackURL
	fallbackConfig.FallbackURL = ""
	return localCoreSelection{
		client:       fallbackClient,
		addr:         fallbackAddr,
		usedFallback: true,
		probeErr:     fallbackClient.probe(ctx),
		launchConfig: fallbackConfig,
	}, nil
}

// ProbeConnection tests the configured local profile without launching a Core
// child or mutating durable state. A successful result includes the endpoint
// actually selected and its API compatibility metadata.
func ProbeConnection(ctx context.Context, cfg Config) (ConnectionDiagnostic, error) {
	selection, err := selectLocalCore(ctx, cfg)
	if err != nil {
		return ConnectionDiagnostic{}, err
	}
	if ctx.Err() != nil {
		return ConnectionDiagnostic{}, ctx.Err()
	}
	if selection.probeErr != nil {
		return ConnectionDiagnostic{}, selection.probeErr
	}
	info, err := selection.client.ConnectionInfo(ctx)
	if err != nil {
		return ConnectionDiagnostic{}, err
	}
	return ConnectionDiagnostic{
		ClientSurface:    "desktop",
		SelectedEndpoint: selection.client.baseURL,
		UsedFallback:     selection.usedFallback,
		ConnectionInfo:   info,
	}, nil
}
