package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"bashfs/internal/packager"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var noValidateFlag bool

func init() {
	packageCmd.Flags().Var(&encodingFlag, "encoding",
		fmt.Sprintf("payload encoding (one of: %s) - base64 trades ~33%% size for copy-paste safety", packager.Encodings))
	if err := packageCmd.RegisterFlagCompletionFunc("encoding",
		cobra.FixedCompletions(packager.Encodings, cobra.ShellCompDirectiveNoFileComp)); err != nil {
		panic(err)
	}
	packageCmd.Flags().BoolVar(&noValidateFlag, "no-validate", false, "skip pre-packaging validation (bash -n, shellcheck, source resolution)")
	rootCmd.AddCommand(packageCmd)
}

var encodingFlag = packager.EncodingRaw // default; overridden by --encoding

var packageCmd = &cobra.Command{
	Use:   "package <script>",
	Short: "Package a bash script with an embedded filesystem (writes to stdout)",
	Long: `Reads a bash script, finds the eval $(bashfs gen <dir>) line, and replaces
it with embedded filesystem data and self-contained helper functions.

The packaged script is written to stdout - redirect with > or pipe it.

Encoding (--encoding):
  raw     (default) raw gzip bytes - smallest, but breaks any text-only round-trip.
          Refuses to run when stdout is a terminal (would splatter binary bytes).
  base64  per-file base64 ASCII - ~33% larger, but survives copy-paste through
          chat clients, web forms, code review comments, and other text-only
          channels that mangle non-printable bytes.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		scriptPath := args[0]

		// Raw mode produces a binary payload - refuse to write that to a
		// terminal so we don't trash the user's cursor/mouse state when
		// they forgot a redirect. Base64 is printable, so it's always OK.
		if encodingFlag == packager.EncodingRaw && term.IsTerminal(int(os.Stdout.Fd())) {
			return fmt.Errorf("refusing to write binary output to terminal; redirect stdout (>) or use --encoding base64")
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

		result, err := packager.Package(string(content), absScriptDir, packager.Options{
			Encoding:       encodingFlag,
			SkipValidation: noValidateFlag,
		})
		if err != nil {
			return err
		}

		if _, err := os.Stdout.Write(result.Data); err != nil {
			return fmt.Errorf("writing to stdout: %w", err)
		}

		fmt.Fprintf(os.Stderr, "bashfs: packaged %s (encoding=%s, %d bytes)\n", scriptPath, encodingFlag, len(result.Data))
		return nil
	},
}

