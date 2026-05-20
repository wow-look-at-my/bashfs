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
	Long: `Generates bash functions (bashfs_cat, bashfs_extract, bashfs_list)
that reference real files on disk. Use with eval in your script during development:

  eval "$(bashfs gen ./myfiles)"

The unquoted form (eval $(bashfs gen ./myfiles)) is also accepted -- the
output is laid out so that bash word-splitting in the unquoted form does not
break the function definitions. The quoted form is still recommended as the
correct shell idiom.`,
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
