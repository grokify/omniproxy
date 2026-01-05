package main

import (
	"fmt"

	"github.com/grokify/omniproxy/pkg/system"
	"github.com/spf13/cobra"
)

func newSystemCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "system",
		Short: "Manage system proxy settings",
		Long:  `Configure the operating system to use OmniProxy.`,
	}

	cmd.AddCommand(
		newSystemSetCmd(),
		newSystemUnsetCmd(),
		newSystemStatusCmd(),
	)

	return cmd
}

type systemSetOptions struct {
	host string
	port int
}

func newSystemSetCmd() *cobra.Command {
	opts := &systemSetOptions{}

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Configure system to use proxy",
		Long: `Configure the operating system to route HTTP/HTTPS traffic through OmniProxy.

On macOS: Configures network services via networksetup
On Windows: Sets registry keys for Internet Settings
On Linux: Configures GNOME/KDE settings and environment variables`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSystemSet(opts)
		},
	}

	cmd.Flags().StringVar(&opts.host, "host", "127.0.0.1", "Proxy host")
	cmd.Flags().IntVarP(&opts.port, "port", "p", 8080, "Proxy port")

	return cmd
}

func runSystemSet(opts *systemSetOptions) error {
	sp, err := system.New()
	if err != nil {
		return fmt.Errorf("unsupported operating system: %w", err)
	}

	fmt.Printf("Configuring system proxy (%s)...\n", sp.Name())
	fmt.Printf("  Host: %s\n", opts.host)
	fmt.Printf("  Port: %d\n", opts.port)

	if err := sp.SetProxy(opts.host, opts.port); err != nil {
		return fmt.Errorf("failed to set proxy: %w", err)
	}

	fmt.Println("\nSystem proxy configured successfully!")
	fmt.Printf("HTTP/HTTPS traffic will now route through %s:%d\n", opts.host, opts.port)
	fmt.Println("\nTo disable, run: omniproxy system unset")

	return nil
}

func newSystemUnsetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unset",
		Short: "Remove system proxy configuration",
		Long:  `Remove the system proxy configuration, restoring direct connections.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSystemUnset()
		},
	}

	return cmd
}

func runSystemUnset() error {
	sp, err := system.New()
	if err != nil {
		return fmt.Errorf("unsupported operating system: %w", err)
	}

	fmt.Printf("Removing system proxy configuration (%s)...\n", sp.Name())

	if err := sp.UnsetProxy(); err != nil {
		return fmt.Errorf("failed to unset proxy: %w", err)
	}

	fmt.Println("System proxy configuration removed!")
	fmt.Println("HTTP/HTTPS traffic will now connect directly.")

	return nil
}

func newSystemStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show current system proxy status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSystemStatus()
		},
	}

	return cmd
}

func runSystemStatus() error {
	sp, err := system.New()
	if err != nil {
		return fmt.Errorf("unsupported operating system: %w", err)
	}

	fmt.Printf("System Proxy Status (%s):\n\n", sp.Name())

	config, err := sp.GetProxy()
	if err != nil {
		return fmt.Errorf("failed to get proxy status: %w", err)
	}

	if config.Enabled {
		fmt.Println("  Status:      Enabled")
		fmt.Printf("  Host:        %s\n", config.Host)
		fmt.Printf("  Port:        %d\n", config.Port)
		if config.HTTPProxy != "" {
			fmt.Printf("  HTTP Proxy:  %s\n", config.HTTPProxy)
		}
		if config.HTTPSProxy != "" {
			fmt.Printf("  HTTPS Proxy: %s\n", config.HTTPSProxy)
		}
	} else {
		fmt.Println("  Status:      Disabled (direct connection)")
	}

	return nil
}
