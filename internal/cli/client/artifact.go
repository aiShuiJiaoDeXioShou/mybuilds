package client

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"mybuilds/internal/protocol"
)

func newArtifactCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "artifact", Short: "读取中央已确认产物", Args: func(_ *cobra.Command, args []string) error {
		if len(args) > 0 {
			return errors.New("产物子命令无效")
		}
		return nil
	}, RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
	list := &cobra.Command{Use: "ls <build-id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		limit, offset, e := remotePage(cmd)
		if e != nil {
			return e
		}
		var result struct {
			Items  []protocol.ArtifactView `json:"items"`
			Limit  int                     `json:"limit"`
			Offset int                     `json:"offset"`
		}
		query := url.Values{"limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}}
		if e = remoteRequest(cmd, http.MethodGet, "/api/builds/"+url.PathEscape(args[0])+"/artifacts?"+query.Encode(), nil, &result, ""); e != nil {
			return e
		}
		rows := make([][]string, 0, len(result.Items))
		for _, v := range result.Items {
			rows = append(rows, []string{v.ID, v.Name, strconv.FormatInt(v.Size, 10), v.SHA256})
		}
		return remoteOutput(cmd, result, []string{"ID", "NAME", "SIZE", "SHA256"}, rows)
	}}
	remotePageFlags(list)
	download := &cobra.Command{Use: "download <artifact-id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		id, e := uuid.Parse(args[0])
		output, _ := cmd.Flags().GetString("output")
		if e != nil || id.String() != args[0] || output == "" {
			return errors.New("产物标识或输出路径无效")
		}
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return downloadRemoteArtifact(ctx, cmd, args[0], output)
	}}
	download.Flags().String("output", "", "目标文件路径，拒绝覆盖")
	cmd.AddCommand(list, download)
	return cmd
}
