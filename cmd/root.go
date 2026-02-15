package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is set at build time.
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "nanobot",
	Short: "nanobot — Ultra-Lightweight Personal AI Assistant (Go edition)",
	Long:  "nanobot-go is a Go rewrite of the nanobot personal AI assistant framework.",
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.Version = Version
}
