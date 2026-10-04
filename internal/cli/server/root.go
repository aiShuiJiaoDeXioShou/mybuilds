package server

import (
	"errors"
	"github.com/spf13/cobra"
	"mybuilds/internal/version"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "mybuilds-server",
		Short:        "移动端构建发布服务端",
		SilenceUsage: true,
	}
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, _ error) error { return errors.New("命令选项不合法") })
	cmd.PersistentFlags().String("config", "", "控制端配置文件")
	cmd.AddCommand(version.NewCommand(), newMigrateCommand(), newGroupCommand(), newTokenCommand(), newProjectCommand(), newServeCommand())
	return cmd
}
