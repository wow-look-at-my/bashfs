package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "bashfs",
	Short: "Embed a filesystem into your bash scripts",
	Long:  "Embed a simple filesystem into your bash scripts, to allow single-file bash script distribution.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
