package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"bashfs/internal/packager"
	"bashfs/internal/profiling"

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
	packageCmd.Flags().Var(&profilingFlag, "profiling-support",
		fmt.Sprintf("embed an opt-in hyperfine profiling mode, run by BASHFS_PROFILE_SCRIPT=1 (one of: %s) - web fetches the harness on demand, local embeds it, none omits it",
			strings.Join(profiling.Supports, ", ")))
	if err := packageCmd.RegisterFlagCompletionFunc("profiling-support",
		cobra.FixedCompletions(profiling.Supports, cobra.ShellCompDirectiveNoFileComp)); err != nil {
		panic(err)
	}
	packageCmd.Flags().BoolVar(&noValidateFlag, "no-validate", false, "skip pre-packaging validation (bash -n, shellcheck, source resolution)")
	rootCmd.AddCommand(packageCmd)
}

var encodingFlag = packager.EncodingRaw // default; overridden by --encoding

var profilingFlag = profiling.SupportWeb // default; overridden by --profiling-support

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
          channels that mangle non-printable bytes.

Profiling (--profiling-support):
  web     (default) embed a tiny stub that downloads a hyperfine profiling
          harness on demand. Adds almost nothing to the script.
  local   embed the whole harness, so profiling works with no network access.
  none    omit profiling support entirely.
  At runtime, set BASHFS_PROFILE_SCRIPT=1 to benchmark the script's bashfs
  operations (startup integrity check + per-file extraction) with hyperfine.`,
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
			Profiling:      profilingFlag,
		})
		if err != nil {
			return err
		}

		if _, err := os.Stdout.Write(result.Data); err != nil {
			return fmt.Errorf("writing to stdout: %w", err)
		}

		fmt.Fprintf(os.Stderr, "bashfs: packaged %s (encoding=%s, profiling=%s, %d bytes)\n", scriptPath, encodingFlag, profilingFlag, len(result.Data))
		return nil
	},
}
