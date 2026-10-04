package client

import (
	"fmt"
	"github.com/spf13/cobra"
	"mybuilds/internal/store"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func newBuildCommand() *cobra.Command {
	build := &cobra.Command{Use: "build", Short: "读取排队记录及步骤进度"}
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
		return remoteOutput(cmd, result, []string{"ID", "PROJECT", "BUILD", "NUMBER", "STATUS", "SHA"}, rows)
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
	build.AddCommand(list, show)
	return build
}
func buildRow(v store.BuildView) []string {
	number := "-"
	if v.Number != nil {
		number = strconv.FormatInt(*v.Number, 10)
	}
	return []string{v.ID, v.Project, v.Name, number, v.Status, v.SHA}
}

func buildDetails(v store.BuildView) [][]string {
	budget := func(value *int64) string {
		if value == nil {
			return "-"
		}
		return strconv.FormatInt(*value, 10)
	}
	rows := [][]string{{"id", v.ID}, {"project", v.Project}, {"group", v.Group}, {"batch_id", v.BatchID}, {"build_name", v.Name}, {"number", budget(v.Number)}, {"status", v.Status}, {"reason", v.Reason}, {"sha", v.SHA}, {"branch", v.Branch}, {"source", v.Source}, {"file", v.File}, {"source_digest", v.SourceDigest}, {"parameter_keys", strings.Join(v.ParameterKeys, ",")}, {"condition", v.Condition}, {"reasons", strings.Join(v.Reasons, ",")}, {"initial_budget_ns", budget(v.InitialBudgetNS)}, {"remaining_budget_ns", budget(v.RemainingBudgetNS)}, {"post_budget_ns", strconv.FormatInt(v.PostBudgetNS, 10)}, {"created_at", v.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")}}
	for _, step := range append(append([]store.StepProgress{}, v.Steps...), v.Post...) {
		prefix := fmt.Sprintf("%s[%d]", step.Phase, step.Index)
		rows = append(rows, []string{prefix, strings.Join([]string{step.Name, step.Kind, step.Condition, step.Status, strings.Join(step.Reasons, ","), strconv.FormatInt(step.ElapsedNS, 10)}, " ")})
	}
	return rows
}
