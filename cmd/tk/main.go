// Copyright (c) 2026 Seb. All rights reserved.

package main

import (
	"fmt"
	"os"

	"github.com/JustSebNL/timekeeper/internal/cli"
)

func main() {
	baseURL, args, err := cli.ResolveBaseURL(os.Args[1:], os.Getenv("TIMEKEEPER_URL"))
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(cli.Run(args, os.Stdout, os.Stderr, baseURL))
}
