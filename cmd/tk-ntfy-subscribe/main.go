// tk-ntfy-subscribe subscribes to an ntfy topic and runs `tk guardian wake`
// when a wake-up message arrives.
//
// This is the agent-side companion to the guardian's ntfy publisher.
// It runs as a supervised service (Bend2Will) and replaces the HTTP receiver
// process that kept dying.
//
// Usage:
//   tk-ntfy-subscribe --server https://ntfy.sh --topic my-tk-topic [--token xxx]
//
// When a message arrives, it runs:
//   tk guardian wake --agent <agent-id>
//
// The agent-id is parsed from the message body (nudge=<id> agent=<id>).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"heckel.io/ntfy/v2/client"
)

func main() {
	server := flag.String("server", "https://ntfy.sh", "ntfy server URL")
	topic := flag.String("topic", "", "ntfy topic to subscribe to")
	token := flag.String("token", "", "ntfy authentication token (optional)")
	agentID := flag.String("agent", "", "agent ID for wake commands (override)")
	flag.Parse()

	if *topic == "" {
		log.Fatal("tk-ntfy-subscribe: --topic is required")
	}

	config := client.NewConfig()
	config.DefaultHost = *server
	if *token != "" {
		config.DefaultToken = *token
	}

	c := client.New(config)

	// Subscribe to the topic.
	subID, err := c.Subscribe(*topic)
	if err != nil {
		log.Fatalf("tk-ntfy-subscribe: subscribe: %v", err)
	}
	defer c.Unsubscribe(subID)

	log.Printf("tk-ntfy-subscribe: listening on %s/%s", *server, *topic)

	// Process messages as they arrive.
	for msg := range c.Messages {
		log.Printf("tk-ntfy-subscribe: message received (topic=%s)", msg.Topic)

		// Parse agent ID from message body.
		aid := *agentID
		if aid == "" {
			aid = parseAgentID(msg.Message)
		}
		if aid == "" {
			log.Printf("tk-ntfy-subscribe: no agent ID in message, skipping")
			continue
		}

		// Run the wake command.
		if err := runWakeCommand(aid); err != nil {
			log.Printf("tk-ntfy-subscribe: wake command failed: %v", err)
		} else {
			log.Printf("tk-ntfy-subscribe: wake completed for agent %s", aid)
		}
	}
}

func parseAgentID(body string) string {
	// Message format: "nudge=<id> agent=<id>"
	parts := strings.Split(body, " ")
	for _, part := range parts {
		if strings.HasPrefix(part, "agent=") {
			return strings.TrimPrefix(part, "agent=")
		}
	}
	return ""
}

func runWakeCommand(agentID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Find tk executable in the same directory as this executable.
	tkName := "tk"
	if runtime.GOOS == "windows" {
		tkName = "tk.exe"
	}
	tkExe := tkName
	if exePath, err := os.Executable(); err == nil {
		tkExe = filepath.Join(filepath.Dir(exePath), tkName)
	}

	cmd := exec.CommandContext(ctx, tkExe, "guardian", "wake", "--agent", agentID)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start wake command: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("wake command: %w", err)
	}

	return nil
}
