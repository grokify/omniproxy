package main

import (
	"fmt"
	"os"

	"github.com/grokify/omniproxy/pkg/ca"
	"github.com/grokify/omniproxy/pkg/system"
	"github.com/spf13/cobra"
)

func newCACmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ca",
		Short: "Manage CA certificates",
		Long:  `Manage CA certificates for MITM proxy.`,
	}

	cmd.AddCommand(
		newCAGenerateCmd(),
		newCAInstallCmd(),
		newCAUninstallCmd(),
		newCAInfoCmd(),
	)

	return cmd
}

type caGenerateOptions struct {
	certPath     string
	keyPath      string
	organization string
	commonName   string
	force        bool
}

func newCAGenerateCmd() *cobra.Command {
	opts := &caGenerateOptions{}

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate a new CA certificate",
		Long: `Generate a new CA certificate for MITM proxy.

The CA certificate and private key will be saved to the specified paths
or the default location (~/.omniproxy/ca/).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCAGenerate(opts)
		},
	}

	cmd.Flags().StringVar(&opts.certPath, "cert", "", "Path to save CA certificate (default: ~/.omniproxy/ca/omniproxy-ca.crt)")
	cmd.Flags().StringVar(&opts.keyPath, "key", "", "Path to save CA private key (default: ~/.omniproxy/ca/omniproxy-ca.key)")
	cmd.Flags().StringVar(&opts.organization, "org", "OmniProxy", "Organization name for the CA")
	cmd.Flags().StringVar(&opts.commonName, "cn", "OmniProxy Root CA", "Common name for the CA")
	cmd.Flags().BoolVarP(&opts.force, "force", "f", false, "Overwrite existing CA")

	return cmd
}

func runCAGenerate(opts *caGenerateOptions) error {
	certPath := opts.certPath
	keyPath := opts.keyPath

	if certPath == "" {
		certPath = ca.DefaultCertPath()
	}
	if keyPath == "" {
		keyPath = ca.DefaultKeyPath()
	}

	cfg := &ca.Config{
		Organization: opts.organization,
		CommonName:   opts.commonName,
	}

	newCA, err := ca.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to generate CA: %w", err)
	}

	if err := newCA.Save(certPath, keyPath); err != nil {
		return fmt.Errorf("failed to save CA: %w", err)
	}

	fmt.Printf("CA certificate generated:\n")
	fmt.Printf("  Certificate: %s\n", certPath)
	fmt.Printf("  Private key: %s\n", keyPath)
	fmt.Printf("\nTo trust this CA, run: omniproxy ca install\n")

	return nil
}

type caInstallOptions struct {
	certPath string
}

func newCAInstallCmd() *cobra.Command {
	opts := &caInstallOptions{}

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install CA certificate into system trust store",
		Long: `Install the CA certificate into the system trust store.

This allows the system and browsers to trust certificates signed by OmniProxy,
enabling HTTPS traffic interception without security warnings.

Note: This may require administrator/sudo privileges.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCAInstall(opts)
		},
	}

	cmd.Flags().StringVar(&opts.certPath, "cert", "", "Path to CA certificate (default: ~/.omniproxy/ca/omniproxy-ca.crt)")

	return cmd
}

func runCAInstall(opts *caInstallOptions) error {
	certPath := opts.certPath
	if certPath == "" {
		certPath = ca.DefaultCertPath()
	}

	sp, err := system.New()
	if err != nil {
		return fmt.Errorf("unsupported operating system: %w", err)
	}

	fmt.Printf("Installing CA certificate into system trust store (%s)...\n", sp.Name())
	fmt.Printf("Certificate: %s\n", certPath)

	if err := sp.InstallCA(certPath, "OmniProxy Root CA"); err != nil {
		fmt.Fprintln(os.Stderr, "\nYou may need to run this command with sudo.")
		return fmt.Errorf("failed to install CA: %w", err)
	}

	fmt.Println("CA certificate installed successfully!")
	fmt.Println("\nNote: You may need to restart your browser for the change to take effect.")

	return nil
}

func newCAUninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove CA certificate from system trust store",
		Long:  `Remove the OmniProxy CA certificate from the system trust store.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCAUninstall()
		},
	}

	return cmd
}

func runCAUninstall() error {
	sp, err := system.New()
	if err != nil {
		return fmt.Errorf("unsupported operating system: %w", err)
	}

	fmt.Printf("Removing CA certificate from system trust store (%s)...\n", sp.Name())

	if err := sp.UninstallCA("OmniProxy Root CA"); err != nil {
		return fmt.Errorf("failed to uninstall CA: %w", err)
	}

	fmt.Println("CA certificate removed successfully!")

	return nil
}

func newCAInfoCmd() *cobra.Command {
	opts := &caInstallOptions{}

	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show CA certificate information",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCAInfo(opts)
		},
	}

	cmd.Flags().StringVar(&opts.certPath, "cert", "", "Path to CA certificate (default: ~/.omniproxy/ca/omniproxy-ca.crt)")

	return cmd
}

func runCAInfo(opts *caInstallOptions) error {
	certPath := opts.certPath
	keyPath := ca.DefaultKeyPath()

	if certPath == "" {
		certPath = ca.DefaultCertPath()
	}

	loadedCA, err := ca.Load(certPath, keyPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "\nRun 'omniproxy ca generate' to create a CA.")
		return fmt.Errorf("failed to load CA: %w", err)
	}

	fmt.Printf("CA Certificate Information:\n")
	fmt.Printf("  File:         %s\n", certPath)
	fmt.Printf("  Subject:      %s\n", loadedCA.Certificate.Subject.String())
	fmt.Printf("  Issuer:       %s\n", loadedCA.Certificate.Issuer.String())
	fmt.Printf("  Not Before:   %s\n", loadedCA.Certificate.NotBefore.Format("2006-01-02 15:04:05"))
	fmt.Printf("  Not After:    %s\n", loadedCA.Certificate.NotAfter.Format("2006-01-02 15:04:05"))
	fmt.Printf("  Serial:       %s\n", loadedCA.Certificate.SerialNumber.String())
	fmt.Printf("  Is CA:        %t\n", loadedCA.Certificate.IsCA)

	// Check if installed
	sp, err := system.New()
	if err == nil {
		installed, _ := sp.IsCAInstalled("OmniProxy Root CA")
		fmt.Printf("  Installed:    %t\n", installed)
	}

	return nil
}
