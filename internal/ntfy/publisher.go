// Package ntfy provides a thin wrapper around the gotfy client for
// TimeKeeper's guardian wake-up mechanism.
//
// The flow:
// 1. Guardian detects an idle agent → publishes ntfy message
// 2. ntfy client service (ntfy subscribe) receives message → runs `tk guardian wake`
// 3. tk guardian wake reads pending events from pulse_events table
// 4. Agent runs work cycle → acknowledges nudge
package ntfy

import (
	"context"
	"fmt"
	"net/url"

	"github.com/AnthonyHewins/gotfy"
)

// Config holds the ntfy publisher configuration.
type Config struct {
	// ServerURL is the ntfy server address (e.g. https://ntfy.sh).
	ServerURL string
	// Topic is the agent's wake-up topic.
	Topic string
	// Token is the optional authentication token.
	Token string
}

// Publisher sends ntfy messages to wake agents.
type Publisher struct {
	config Config
	client *gotfy.Publisher
}

// NewPublisher creates an ntfy publisher for the given config.
func NewPublisher(config Config) (*Publisher, error) {
	if config.ServerURL == "" {
		return nil, fmt.Errorf("ntfy server URL is required")
	}
	if config.Topic == "" {
		return nil, fmt.Errorf("ntfy topic is required")
	}
	server, err := url.Parse(config.ServerURL)
	if err != nil {
		return nil, fmt.Errorf("parse ntfy server URL: %w", err)
	}
	var opts []gotfy.Option
	if config.Token != "" {
		opts = append(opts, gotfy.WithAuth("", config.Token))
	}
	client, err := gotfy.NewPublisher(server, opts...)
	if err != nil {
		return nil, fmt.Errorf("create ntfy publisher: %w", err)
	}
	return &Publisher{config: config, client: client}, nil
}

// WakeAgent publishes a wake-up message to the agent's ntfy topic.
// The message contains the nudge ID so the agent can ack it after work.
func (p *Publisher) WakeAgent(ctx context.Context, nudgeID int64, agentID string) error {
	msg := &gotfy.Message{
		Topic:    p.config.Topic,
		Message:  fmt.Sprintf("nudge=%d agent=%s", nudgeID, agentID),
		Title:    "TimeKeeper Guardian Wake",
		Tags:     []string{"wake", agentID},
		Priority: gotfy.High,
	}
	_, err := p.client.SendMessage(ctx, msg)
	return err
}

// Close closes the publisher.
func (p *Publisher) Close() error {
	return nil // gotfy publisher has no close
}
