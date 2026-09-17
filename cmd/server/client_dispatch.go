package main

import (
	"fmt"
	"github.com/JustSebNL/timekeeper/internal/cli"
	"io"
	"strings"
)

// dispatchClient routes positional client commands before server configuration
// can create files or bind ports. Server flags keep their existing semantics.
func dispatchClient(args []string, environmentURL string, out, errOut io.Writer) (bool, int) {
	if len(args) == 0 {
		return false, 0
	}
	if strings.HasPrefix(args[0], "-") && args[0] != "--url" && !strings.HasPrefix(args[0], "--url=") {
		return false, 0
	}
	baseURL, resolved, err := cli.ResolveBaseURL(args, environmentURL)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return true, 2
	}
	return true, cli.Run(resolved, out, errOut, baseURL)
}
