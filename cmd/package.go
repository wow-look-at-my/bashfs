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
	packageCmd.Flags().Var(&encodingFlag, "encoding",
		fmt.Sprintf("payload encoding (one of: %s) — base64 trades ~33%% size for copy-paste safety", packager.Encodings))
	if err := packageCmd.RegisterFlagCompletionFunc("encoding",
		cobra.FixedCompletions(packager.Encodings, cobra.ShellCompDirectiveNoFileComp)); err != nil {
		// Registration only fails if the flag doesn't exist — we just
		// declared it, so this is a programmer error.
		panic(err)
	}
	packageCmd.MarkFlagRequired("output")
	rootCmd.AddCommand(packageCmd)
}

var (
	outputFlag   string
	encodingFlag = packager.EncodingRaw // default; overridden by --encoding
)

var packageCmd = &cobra.Command{
	Use:   "package <script>",
	Short: "Package a bash script with an embedded filesystem",
	Long: `Reads a bash script, finds the eval $(bashfs gen <dir>) line, and replaces
it with embedded filesystem data and self-contained helper functions.

The output is a single, distributable bash script with no external file dependencies.

The trailing payload encoding is controlled by --encoding:
  raw     (default) raw gzip bytes — smallest, but breaks any text-only round-trip
  base64  per-file base64 ASCII — ~33% larger, but survives copy-paste through
          chat clients, web forms, code review comments, and other text-only
          channels that mangle non-printable bytes.`,
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

		result, err := packager.Package(string(content), absScriptDir, packager.Options{Encoding: encodingFlag})
		if err != nil {
			return err
		}

		if err := os.WriteFile(outputFlag, result.Data, 0755); err != nil {
			return fmt.Errorf("writing output %s: %w", outputFlag, err)
		}

		fmt.Fprintf(os.Stderr, "bashfs: packaged %s -> %s (encoding=%s)\n", scriptPath, outputFlag, encodingFlag)
		return nil
	},
}
