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
	remoteRootFlags(cmd)
	cmd.AddCommand(version.NewCommand(), newInitCommand(), newRunCommand(), newDoctorCommand(), newRemoteGroupCommand(), newBuildCommand(), newStatusCommand(), newRemoteProjectCommand(), newTriggerCommand())
	return cmd
}
