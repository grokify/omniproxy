package main

import (
	"fmt"

	"github.com/grokify/omniproxy/pkg/config"
	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
		Long:  `Manage OmniProxy configuration files.`,
	}

	cmd.AddCommand(
		newConfigInitCmd(),
		newConfigShowCmd(),
	)

	return cmd
}

type configInitOptions struct {
	output string
	force  bool
}

func newConfigInitCmd() *cobra.Command {
	opts := &configInitOptions{}

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a configuration file",
		Long: `Create a new configuration file with default settings.

The configuration file uses YAML format and includes all available options
with their default values.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigInit(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.output, "output", "o", "", "Output path (default: ~/.omniproxy/config.yaml)")
	cmd.Flags().BoolVarP(&opts.force, "force", "f", false, "Overwrite existing file")

	return cmd
}

func runConfigInit(opts *configInitOptions) error {
	path := opts.output
	if path == "" {
		path = config.DefaultConfigPath()
	}

	cfg := config.DefaultConfig()

	// Add some example values
	cfg.Capture.Output = "traffic.ndjson"
	cfg.Filter.IncludeHosts = []string{"api.example.com"}
	cfg.MITM.SkipHosts = []string{"*.pinned-app.com"}

	if err := cfg.Save(path); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Configuration file created: %s\n", path)
	fmt.Printf("\nTo use this configuration:\n")
	fmt.Printf("  omniproxy serve --config %s\n", path)

	return nil
}

func newConfigShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show example configuration",
		Long:  `Display an example configuration file with all available options.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigShow()
		},
	}

	return cmd
}

func runConfigShow() error {
	fmt.Println("# OmniProxy Configuration Example")
	fmt.Println("#")
	fmt.Println("# Save this to ~/.omniproxy/config.yaml or specify with --config flag")
	fmt.Println()
	fmt.Println(config.ExampleConfig())
	return nil
}
