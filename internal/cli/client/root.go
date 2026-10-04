package client

import (
	"errors"
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
	cmd.Args = func(_ *cobra.Command, args []string) error {
		if len(args) > 0 {
			return errors.New("命令无效")
		}
		return nil
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error { return cmd.Help() }
	cmd.AddCommand(version.NewCommand(), newInitCommand(), newRunCommand(), newDoctorCommand(), newRemoteGroupCommand(), newBuildCommand(), newStatusCommand(), newRemoteProjectCommand(), newTriggerCommand(), newRemoteNodeCommand(), newLogsCommand(), newArtifactCommand())
	return cmd
}
