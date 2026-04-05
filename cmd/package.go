package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"bashfs/internal/packager"

	"github.com/spf13/cobra"
)

func init() {
	packageCmd.Flags().StringVarP(&outputFlag, "output", "o", "", "output file path (required)")
	packageCmd.MarkFlagRequired("output")
	rootCmd.AddCommand(packageCmd)
}

var outputFlag string

var packageCmd = &cobra.Command{
	Use:   "package <script>",
	Short: "Package a bash script with an embedded filesystem",
	Long: `Reads a bash script, finds the eval $(bashfs gen <dir>) line, and replaces
it with embedded filesystem data and self-contained helper functions.

The output is a single, distributable bash script with no external file dependencies.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		scriptPath := args[0]

		content, err := os.ReadFile(scriptPath)
		if err != nil {
			return fmt.Errorf("reading script %s: %w", scriptPath, err)
		}

		scriptDir := filepath.Dir(scriptPath)
		absScriptDir, err := filepath.Abs(scriptDir)
		if err != nil {
			return fmt.Errorf("resolving script directory: %w", err)
		}

		result, err := packager.Package(string(content), absScriptDir)
		if err != nil {
			return err
		}

		if err := os.WriteFile(outputFlag, []byte(result), 0755); err != nil {
			return fmt.Errorf("writing output %s: %w", outputFlag, err)
		}

		fmt.Fprintf(os.Stderr, "bashfs: packaged %s -> %s\n", scriptPath, outputFlag)
		return nil
	},
}
