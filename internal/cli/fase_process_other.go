//go:build !windows && !linux

package cli

import "fmt"

func processSessionSource() (string, error) {
	return "", fmt.Errorf("runtime must expose TIMEKEEPER_SESSION_ID on this platform; enrollment itself is automatic")
}
