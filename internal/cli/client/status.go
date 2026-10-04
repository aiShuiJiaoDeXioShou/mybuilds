package client

import (
	"github.com/spf13/cobra"
	"mybuilds/internal/server"
	"net/http"
	"strconv"
)

func newStatusCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "status", Short: "查看控制端排队状态", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		var result server.StatusDTO
		if err := remoteRequest(cmd, http.MethodGet, "/api/status", nil, &result, ""); err != nil {
			return err
		}
		return remoteOutput(cmd, result, []string{"VERSION", "PROJECTS", "QUEUED", "SKIPPED", "RUNNING", "NODES"}, [][]string{{result.Version, strconv.FormatInt(result.Projects, 10), strconv.FormatInt(result.Queued, 10), strconv.FormatInt(result.Skipped, 10), strconv.FormatInt(result.Running, 10), strconv.FormatInt(result.Nodes, 10)}})
	}}
	cmd.Flags().Bool("json", false, "输出JSON")
	return cmd
}
