package agent

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	node "mybuilds/internal/agent"
	"mybuilds/internal/config"
)

func newServeCommand() *cobra.Command {
	return &cobra.Command{Use: "serve", Short: "连接控制端并启动独立节点", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		filename, _ := cmd.Flags().GetString("config")
		cfg, err := config.LoadAgent(config.AgentLoadOptions{Filename: filename, Explicit: cmd.Flags().Changed("config")})
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return node.Serve(ctx, cfg)
	}}
}
