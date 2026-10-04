package version

import (
	"fmt"

	"github.com/spf13/cobra"
)

// 构建时通过 -ldflags -X 注入，两端共享默认值。
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func NewCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "显示版本与构建信息",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s (commit: %s, built: %s)\n", Version, Commit, BuildDate)
			return err
		},
	}
}
