package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift/oadp-operator/must-gather/pkg"
)

func init() {
	pkg.CLI.Flags().DurationVarP(&pkg.RequestTimeout, "request-timeout", "r", pkg.DefaultRequestTimeout, "Timeout per OADP server request (like collecting logs from a backup)")
	pkg.CLI.Flags().BoolVarP(&pkg.SkipTLS, "skip-tls", "s", false, "Run OADP server requests with insecure TLS connections (recommended if a custom CA certificate is used) (default false)")
	// TODO caCertFile?
	pkg.CLI.Flags().BoolP("help", "h", false, "Show OADP Must-gather help message.")

	pkg.CLI.SetHelpCommand(&cobra.Command{Hidden: true, Use: ""})
}

func main() {
	// TODO JSON output in the future?
	err := pkg.CLI.Execute()
	if err != nil {
		os.Exit(1)
	}
}
