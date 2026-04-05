package cmd

import (
	"fmt"
	"os"

	"bashfs/internal/bashgen"
	"bashfs/internal/fswalker"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(genCmd)
}

var genCmd = &cobra.Command{
	Use:   "gen <dir>",
	Short: "Generate bash helper functions backed by real files (for development)",
	Long: `Generates bash functions (bashfs_cat, bashfs_extract, bashfs_list, bashfs_jq)
that reference real files on disk. Use with eval in your script during development:

  eval $(bashfs gen ./myfiles)`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := args[0]

		info, err := os.Stat(dir)
		if err != nil {
			return fmt.Errorf("cannot access directory %s: %w", dir, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("%s is not a directory", dir)
		}

		files, err := fswalker.Walk(dir)
		if err != nil {
			return err
		}

		output, err := bashgen.GenerateDevMode(files, dir)
		if err != nil {
			return err
		}

		fmt.Print(output)
		return nil
	},
}
