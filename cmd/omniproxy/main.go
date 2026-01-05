// OmniProxy is a universal HTTP/HTTPS proxy with MITM support for traffic capture.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "0.1.0"

func main() {
	rootCmd := &cobra.Command{
		Use:   "omniproxy",
		Short: "Universal HTTP/HTTPS proxy with MITM support",
		Long: `OmniProxy is a universal proxy for capturing HTTP/HTTPS traffic.

It supports:
  - HTTP forward proxy
  - HTTPS MITM (Man-in-the-Middle) for traffic inspection
  - Traffic capture in multiple formats (HAR, IR, NDJSON)
  - System proxy configuration for macOS, Windows, and Linux
  - Automatic CA certificate management`,
		Version: version,
	}

	rootCmd.AddCommand(
		newServeCmd(),
		newReverseCmd(),
		newDaemonCmd(),
		newCACmd(),
		newSystemCmd(),
		newConfigCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
