package client

import (
	"errors"
	"github.com/spf13/cobra"
	"mybuilds/internal/store"
	"net/http"
	"net/url"
	"strconv"
)

func newApprovalsCommand() *cobra.Command {
	var project, state string
	var limit, offset int
	cmd := &cobra.Command{Use: "approvals", Short: "查看原构建的审批证据", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
			return errors.New("审批分页参数无效")
		}
		query := url.Values{"project_id": {project}, "state": {state}, "limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}}
		var items []store.ApprovalView
		if err := remoteRequest(cmd, http.MethodGet, "/api/approvals?"+query.Encode(), nil, &items, ""); err != nil {
			return err
		}
		return approvalOutput(cmd, items, items)
	}}
	cmd.Flags().StringVar(&project, "project-id", "", "项目UUID")
	cmd.Flags().StringVar(&state, "state", "", "筛选审批状态")
	cmd.Flags().IntVar(&limit, "limit", 20, "每页最多100项")
	cmd.Flags().IntVar(&offset, "offset", 0, "分页偏移")
	cmd.Flags().Bool("json", false, "输出JSON")
	return cmd
}
func newApprovalDecisionCommand(decision string) *cobra.Command {
	var id, digest, note string
	var revision int64
	cmd := &cobra.Command{Use: decision + " <build>", Short: map[string]string{"approve": "批准精确原审批", "reject": "拒绝精确原审批"}[decision], Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if id == "" || digest == "" || revision < 1 {
			return errors.New("需要精确审批ID、revision与checkpoint摘要")
		}
		input := struct {
			ApprovalID       string `json:"approval_id"`
			Revision         int64  `json:"revision"`
			CheckpointDigest string `json:"checkpoint_digest"`
			Note             string `json:"note"`
		}{id, revision, digest, note}
		var view store.ApprovalView
		if err := remoteRequest(cmd, http.MethodPost, "/api/builds/"+url.PathEscape(args[0])+"/"+decision, input, &view, ""); err != nil {
			return err
		}
		return approvalOutput(cmd, view, []store.ApprovalView{view})
	}}
	cmd.Flags().StringVar(&id, "approval-id", "", "精确审批UUID")
	cmd.Flags().Int64Var(&revision, "revision", 0, "原审批revision")
	cmd.Flags().StringVar(&digest, "checkpoint-digest", "", "原checkpoint摘要")
	cmd.Flags().StringVar(&note, "note", "", "有限安全审计备注")
	cmd.Flags().Bool("json", false, "输出JSON")
	return cmd
}
func approvalOutput(cmd *cobra.Command, value any, items []store.ApprovalView) error {
	rows := [][]string{}
	for _, v := range items {
		rows = append(rows, []string{v.ID, v.BuildID, v.Step, strconv.FormatInt(v.Revision, 10), v.State, v.NodeName, v.CheckpointDigest})
	}
	return remoteOutput(cmd, value, []string{"APPROVAL", "BUILD", "STEP", "REVISION", "STATE", "NODE", "CHECKPOINT"}, rows)
}
