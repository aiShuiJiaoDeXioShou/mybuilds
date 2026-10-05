package client

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"mybuilds/internal/store"
)

func newRetentionCommand() *cobra.Command {
	retention := &cobra.Command{
		Use: "retention", Short: "管理项目历史保留策略",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				return errors.New("保留策略子命令无效")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	show := &cobra.Command{Use: "show <project>", Short: "查看有效策略和字段来源", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var view store.EffectiveRetention
		if err := remoteRequest(cmd, http.MethodGet, "/api/projects/"+url.PathEscape(args[0])+"/retention", nil, &view, ""); err != nil {
			return err
		}
		return remoteOutput(cmd, view,
			[]string{"BUILDS", "DAYS", "BUILDS_SOURCE", "DAYS_SOURCE", "GLOBAL_VERSION", "PROJECT_VERSION"},
			[][]string{{strconv.FormatInt(view.Builds, 10), strconv.FormatInt(view.Days, 10), view.BuildsSource, view.DaysSource, strconv.FormatInt(view.GlobalVersion, 10), strconv.FormatInt(view.ProjectVersion, 10)}})
	}}
	show.Flags().Bool("json", false, "输出JSON")
	list := &cobra.Command{Use: "ls <project>", Short: "查看历史清理事项或只读候选评估", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		limit, offset, err := remotePage(cmd)
		if err != nil {
			return err
		}
		candidates, _ := cmd.Flags().GetBool("candidates")
		endpoint := "jobs"
		if candidates {
			endpoint = "candidates"
		}
		query := url.Values{"limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}}
		var result store.RetentionPage
		if err = remoteRequest(cmd, http.MethodGet, "/api/projects/"+url.PathEscape(args[0])+"/retention/"+endpoint+"?"+query.Encode(), nil, &result, ""); err != nil {
			return err
		}
		return retentionPageOutput(cmd, result)
	}}
	list.Flags().Bool("candidates", false, "只评估数量、时间条件及保护原因，不启动清理")
	remotePageFlags(list)

	run := &cobra.Command{Use: "run <project>", Short: "有界推进中央清理并查看实际分态", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		limit, _ := cmd.Flags().GetInt("limit")
		if limit < 1 || limit > 100 {
			return errors.New("清理批量不合法")
		}
		body := struct {
			Limit int `json:"limit"`
		}{Limit: limit}
		var result store.RetentionPage
		if err := remoteRequest(cmd, http.MethodPost, "/api/projects/"+url.PathEscape(args[0])+"/retention", body, &result, ""); err != nil {
			return err
		}
		return retentionPageOutput(cmd, result)
	}}
	run.Flags().Int("limit", 100, "本轮最多推进100项")
	run.Flags().Bool("json", false, "输出JSON")
	retention.AddCommand(show, list, run)
	return retention
}

// run与ls共享实际事项输出，不筛掉未完成或受保护记录。
func retentionPageOutput(cmd *cobra.Command, result store.RetentionPage) error {
	rows := make([][]string, 0, len(result.Items))
	for _, entry := range result.Items {
		number := ""
		if entry.Number != nil {
			number = strconv.FormatInt(*entry.Number, 10)
		}
		rows = append(rows, []string{entry.BuildID, entry.BuildName, number, entry.HistoryState, strconv.FormatBool(entry.Candidate), strings.Join(entry.ProtectReasons, ","), entry.JobID, entry.CentralState, entry.NodeState, entry.Reason})
	}
	return remoteOutput(cmd, result, []string{"BUILD_ID", "BUILD", "NUMBER", "HISTORY_STATE", "CANDIDATE", "PROTECT_REASONS", "JOB_ID", "CENTRAL_STATE", "NODE_STATE", "REASON"}, rows)
}
