package client

import (
	"github.com/spf13/cobra"
	"mybuilds/internal/version"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "mybuilds",
		Short:        "移动端构建发布客户端",
		SilenceUsage: true,
	}
	cmd.AddCommand(version.NewCommand())
	return cmd
}
