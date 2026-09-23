package ntfy

import (
	"context"
	"fmt"
	"os"
	"time"
)

// RunPublish publishes a wake-up message via ntfy.
func RunPublish(serverURL, topic, token string, nudgeID int64, agentID string) error {
	publisher, err := NewPublisher(Config{
		ServerURL: serverURL,
		Topic:      topic,
		Token:      token,
	})
	if err != nil {
		return err
	}
	defer publisher.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := publisher.WakeAgent(ctx, nudgeID, agentID); err != nil {
		return fmt.Errorf("publish ntfy wake: %w", err)
	}

	fmt.Fprintf(os.Stderr, "ntfy wake published: nudge=%d agent=%s topic=%s\n", nudgeID, agentID, topic)
	return nil
}
