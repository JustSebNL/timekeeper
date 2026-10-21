// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package cli

import (
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// CobraEngine wraps TimeKeeper CLI with spf13/cobra commands and spf13/viper configuration.
type CobraEngine struct {
	Viper   *viper.Viper
	RootCmd *cobra.Command
	Out     io.Writer
	ErrOut  io.Writer
	BaseURL string
}

// NewCobraEngine initializes the Cobra command tree and Viper environment binding.
func NewCobraEngine(args []string, out, errOut io.Writer, defaultBaseURL string) *CobraEngine {
	v := viper.New()
	v.SetEnvPrefix("TIMEKEEPER")
	v.AutomaticEnv()
	v.SetDefault("url", defaultBaseURL)
	v.SetDefault("json", false)

	engine := &CobraEngine{
		Viper:   v,
		Out:     out,
		ErrOut:  errOut,
		BaseURL: defaultBaseURL,
	}

	rootCmd := &cobra.Command{
		Use:   "tk",
		Short: "TimeKeeper CLI — Task & Sprint Management System",
		Long:  "TimeKeeper CLI is the command line boundary for managing projects, tasks, sprints, handoffs, and event streams.",
		RunE: func(cmd *cobra.Command, cmdArgs []string) error {
			if len(cmdArgs) == 0 {
				return cmd.Help()
			}
			return nil
		},
	}

	rootCmd.SetArgs(args)
	rootCmd.SetOut(out)
	rootCmd.SetErr(errOut)

	rootCmd.PersistentFlags().StringVar(&engine.BaseURL, "url", defaultBaseURL, "TimeKeeper Core server URL")
	rootCmd.PersistentFlags().Bool("json", false, "Output responses in machine-readable JSON format")

	_ = v.BindPFlag("url", rootCmd.PersistentFlags().Lookup("url"))
	_ = v.BindPFlag("json", rootCmd.PersistentFlags().Lookup("json"))

	engine.RootCmd = rootCmd
	return engine
}

// ExecuteCobra executes the CLI using Cobra command parsing and Viper configuration bindings.
func ExecuteCobra(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 0 {
		args = []string{"--help"}
	}

	// Read environment override if specified
	if envURL := os.Getenv("TIMEKEEPER_BASE_URL"); envURL != "" {
		baseURL = envURL
	}

	engine := NewCobraEngine(args, out, errOut, baseURL)

	// Execute command dispatched via RunTracked to preserve exact FASE identity and API behavior
	return RunTracked(args, out, errOut, engine.BaseURL)
}

// BuildCobraTree generates the explicit Cobra subcommand hierarchy for diagnostic inspection.
func BuildCobraTree(out, errOut io.Writer, baseURL string) *cobra.Command {
	engine := NewCobraEngine(nil, out, errOut, baseURL)

	subcommands := []struct {
		use   string
		short string
	}{
		{"list", "List all active projects"},
		{"tree", "Show project execution hierarchy"},
		{"handoff", "Generate agent handoff snapshot"},
		{"project", "Project management commands"},
		{"task", "Task management commands"},
		{"sprint", "Sprint management commands"},
		{"tc", "Project-scoped Treasure Chest for reusable gems"},
		{"events", "Event stream commands"},
		{"doctor", "System health diagnostics"},
	}

	for _, sc := range subcommands {
		cmd := &cobra.Command{
			Use:   sc.use,
			Short: sc.short,
			Run: func(cmd *cobra.Command, args []string) {
				fullArgs := append([]string{cmd.Use}, args...)
				_ = RunTracked(fullArgs, out, errOut, baseURL)
			},
		}
		engine.RootCmd.AddCommand(cmd)
	}

	return engine.RootCmd
}
