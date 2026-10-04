package server

import (
	"github.com/spf13/cobra"
	"mybuilds/internal/version"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "mybuilds-server",
		Short:        "移动端构建发布服务端",
		SilenceUsage: true,
	}
	cmd.AddCommand(version.NewCommand())
	return cmd
}
