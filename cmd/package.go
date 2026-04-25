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
	packageCmd.Flags().StringVar(&encodingFlag, "encoding", "raw",
		"payload encoding: 'raw' (binary, smallest) or 'base64' (printable ASCII, copy-paste safe)")
	packageCmd.MarkFlagRequired("output")
	rootCmd.AddCommand(packageCmd)
}

var (
	outputFlag   string
	encodingFlag string
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

		enc, err := parseEncoding(encodingFlag)
		if err != nil {
			return err
		}

		content, err := os.ReadFile(scriptPath)
		if err != nil {
			return fmt.Errorf("reading script %s: %w", scriptPath, err)
		}

		scriptDir := filepath.Dir(scriptPath)
		absScriptDir, err := filepath.Abs(scriptDir)
		if err != nil {
			return fmt.Errorf("resolving script directory: %w", err)
		}

		result, err := packager.Package(string(content), absScriptDir, packager.Options{Encoding: enc})
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

func parseEncoding(s string) (packager.Encoding, error) {
	switch s {
	case "raw":
		return packager.EncodingRaw, nil
	case "base64":
		return packager.EncodingBase64, nil
	default:
		return 0, fmt.Errorf("unknown encoding %q (expected 'raw' or 'base64')", s)
	}
}
