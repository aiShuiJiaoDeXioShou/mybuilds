package client

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"unicode"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

func addBuildStopCommands(build *cobra.Command) {
	cancel := &cobra.Command{Use: "cancel <id>", Short: "保存构建取消意图", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var result store.BuildView
		if e := remoteRequest(cmd, http.MethodPost, "/api/builds/"+url.PathEscape(args[0])+"/cancel", nil, &result, ""); e != nil {
			return e
		}
		return remoteOutput(cmd, result, []string{"FIELD", "VALUE"}, buildDetails(result))
	}}
	cancel.Flags().Bool("json", false, "输出JSON")
	confirm := &cobra.Command{Use: "confirm-stopped <id>", Short: "记录管理员实际观察到的停止依据", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		in := protocol.StopConfirmation{EvidenceCode: "admin_observed_stopped"}
		in.Ref.BuildID = args[0]
		for flag, target := range map[string]*string{"node-id": &in.Ref.NodeID, "session": &in.Ref.SessionID, "attempt": &in.Ref.AttemptID, "lease": &in.Ref.LeaseID} {
			if !cmd.Flags().Changed(flag) {
				return errors.New("停止确认缺少完整执行标识")
			}
			*target, _ = cmd.Flags().GetString(flag)
		}
		if !cmd.Flags().Changed("epoch") || !cmd.Flags().Changed("note") {
			return errors.New("停止确认缺少完整执行标识或依据")
		}
		in.Ref.Epoch, _ = cmd.Flags().GetInt64("epoch")
		in.Note, _ = cmd.Flags().GetString("note")
		if in.Ref.Epoch < 1 || len(in.Note) < 1 || len(in.Note) > 1024 {
			return errors.New("停止确认无效")
		}
		for _, id := range []string{in.Ref.NodeID, in.Ref.SessionID, in.Ref.BuildID, in.Ref.AttemptID, in.Ref.LeaseID} {
			parsed, e := uuid.Parse(id)
			if e != nil || parsed.String() != id {
				return errors.New("停止确认无效")
			}
		}
		for _, r := range in.Note {
			if unicode.IsControl(r) {
				return errors.New("停止确认无效")
			}
		}
		if e := remoteRequest(cmd, http.MethodPost, "/api/builds/"+url.PathEscape(args[0])+"/stop-confirmation", in, nil, ""); e != nil {
			return e
		}
		if _, e := fmt.Fprintln(cmd.OutOrStdout(), "停止依据已记录"); e != nil {
			return errors.New("写入API结果失败")
		}
		return nil
	}}
	for _, flag := range []string{"node-id", "session", "attempt", "lease"} {
		confirm.Flags().String(flag, "", "原执行标识")
	}
	confirm.Flags().Int64("epoch", 0, "原租约epoch")
	confirm.Flags().String("note", "", "实际观察到的停止依据")
	build.AddCommand(cancel, confirm)
}
