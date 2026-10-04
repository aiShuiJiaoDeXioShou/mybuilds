package client

import (
	"errors"
	"fmt"
	"github.com/spf13/cobra"
	"mybuilds/internal/store"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func newBuildCommand() *cobra.Command {
	build := &cobra.Command{Use: "build", Short: "读取构建记录及步骤进度", Args: func(_ *cobra.Command, args []string) error {
		if len(args) != 0 {
			return errors.New("构建子命令无效")
		}
		return nil
	}, RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
	list := &cobra.Command{Use: "ls", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		limit, offset, err := remotePage(cmd)
		if err != nil {
			return err
		}
		query := url.Values{"limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}}
		for flag, key := range map[string]string{"project": "project", "group": "group", "build-name": "build_name", "batch": "batch_id", "status": "status"} {
			value, _ := cmd.Flags().GetString(flag)
			if value != "" {
				query.Set(key, value)
			}
		}
		var result struct {
			Items  []store.BuildView `json:"items"`
			Limit  int               `json:"limit"`
			Offset int               `json:"offset"`
		}
		if err := remoteRequest(cmd, http.MethodGet, "/api/builds?"+query.Encode(), nil, &result, ""); err != nil {
			return err
		}
		rows := make([][]string, 0, len(result.Items))
		for _, v := range result.Items {
			rows = append(rows, buildRow(v))
		}
		return remoteOutput(cmd, result, []string{"ID", "PROJECT", "BUILD", "NUMBER", "STATUS", "NODE", "REASON", "RETRY_OF", "SHA"}, rows)
	}}
	remotePageFlags(list)
	for _, flag := range []string{"project", "group", "build-name", "batch", "status"} {
		list.Flags().String(flag, "", "过滤条件")
	}
	show := &cobra.Command{Use: "show <id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var result store.BuildView
		if err := remoteRequest(cmd, http.MethodGet, "/api/builds/"+url.PathEscape(args[0]), nil, &result, ""); err != nil {
			return err
		}
		return remoteOutput(cmd, result, []string{"FIELD", "VALUE"}, buildDetails(result))
	}}
	show.Flags().Bool("json", false, "输出JSON")
	build.AddCommand(list, show, newRetryCommand())
	addBuildStopCommands(build)
	return build
}
func buildRow(v store.BuildView) []string {
	number := "-"
	if v.Number != nil {
		number = strconv.FormatInt(*v.Number, 10)
	}
	return []string{v.ID, v.Project, v.Name, number, v.Status, v.NodeName, v.Reason, v.RetryOf, v.SHA}
}

func buildDetails(v store.BuildView) [][]string {
	budget := func(value *int64) string {
		if value == nil {
			return "-"
		}
		return strconv.FormatInt(*value, 10)
	}
	rows := [][]string{{"id", v.ID}, {"project", v.Project}, {"group", v.Group}, {"batch_id", v.BatchID}, {"build_name", v.Name}, {"number", budget(v.Number)}, {"status", v.Status}, {"reason", v.Reason}, {"sha", v.SHA}, {"branch", v.Branch}, {"source", v.Source}, {"file", v.File}, {"source_digest", v.SourceDigest}, {"parameter_keys", strings.Join(v.ParameterKeys, ",")}, {"condition", v.Condition}, {"reasons", strings.Join(v.Reasons, ",")}, {"initial_budget_ns", budget(v.InitialBudgetNS)}, {"remaining_budget_ns", budget(v.RemainingBudgetNS)}, {"post_budget_ns", strconv.FormatInt(v.PostBudgetNS, 10)}, {"created_at", v.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")}}
	rows = append(rows, [][]string{{"node_id", v.NodeID}, {"node_name", v.NodeName}, {"session_id", v.SessionID}, {"attempt_id", v.AttemptID}, {"lease_id", v.LeaseID}, {"lease_epoch", strconv.FormatInt(v.LeaseEpoch, 10)}, {"cancel_requested", strconv.FormatBool(v.CancelRequested)}, {"stop_unconfirmed", strconv.FormatBool(v.StopUnconfirmed)}, {"remaining_post_budget_ns", strconv.FormatInt(v.RemainingPostBudgetNS, 10)}, {"post_phase", v.PostPhase}}...)
	if v.RetryOf != "" {
		rows = append(rows, []string{"retry_of", v.RetryOf})
	}
	for _, step := range append(append([]store.StepProgress{}, v.Steps...), v.Post...) {
		prefix := fmt.Sprintf("%s[%d]", step.Phase, step.Index)
		rows = append(rows, []string{prefix, strings.Join([]string{step.Name, step.Kind, step.Condition, step.Status, strings.Join(step.Reasons, ","), "elapsed_ns=" + strconv.FormatInt(step.ElapsedNS, 10), "intent=" + strconv.FormatBool(step.Intent), "started=" + strconv.FormatBool(step.Started), "stop_confirmed=" + strconv.FormatBool(step.StopConfirmed), "cleanup_failed=" + strconv.FormatBool(step.CleanupFailed), "reason=" + step.Reason, "exit_code=" + strconv.Itoa(step.ExitCode)}, " ")})
	}
	return rows
}
