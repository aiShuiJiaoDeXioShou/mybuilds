package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
	"mybuilds/internal/config"
	"mybuilds/internal/store"
)

func newPublishCommand() *cobra.Command {
	root := &cobra.Command{Use: "publish", Short: "查看商店发布及核对原动作", RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
	list := &cobra.Command{Use: "ls", Short: "查看发布记录", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		project, _ := cmd.Flags().GetString("project")
		after, _ := cmd.Flags().GetString("after")
		limit, _ := cmd.Flags().GetInt("limit")
		if limit < 1 || limit > 100 {
			return errors.New("发布分页参数无效")
		}
		query := url.Values{"project": {project}, "after": {after}, "limit": {strconv.Itoa(limit)}}
		var result struct {
			Items []store.PublishView `json:"items"`
			Limit int                 `json:"limit"`
		}
		if err := remoteRequest(cmd, http.MethodGet, "/api/publishes?"+query.Encode(), nil, &result, ""); err != nil {
			return err
		}
		return publishesOutput(cmd, result, result.Items)
	}}
	list.Flags().String("project", "", "按项目筛选")
	list.Flags().String("after", "", "继续上次发布ID之后的页")
	list.Flags().Int("limit", 20, "每页最多100项")
	list.Flags().Bool("json", false, "输出JSON")
	show := &cobra.Command{Use: "show <intent>", Short: "查看原发布动作和保护状态", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var view store.PublishView
		if err := remoteRequest(cmd, http.MethodGet, "/api/publishes/"+url.PathEscape(args[0]), nil, &view, ""); err != nil {
			return err
		}
		return publishesOutput(cmd, view, []store.PublishView{view})
	}}
	show.Flags().Bool("json", false, "输出JSON")
	query := &cobra.Command{Use: "query <intent>", Short: "请求原节点只读核对，不重传", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var view store.PublishQueryView
		if err := remoteRequest(cmd, http.MethodPost, "/api/publishes/"+url.PathEscape(args[0])+"/query", nil, &view, ""); err != nil {
			return err
		}
		return publishQueryOutput(cmd, view)
	}}
	query.Flags().Bool("json", false, "输出JSON")
	queryShow := &cobra.Command{Use: "query-show <query>", Short: "查看核对结果与有限远端事实", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var view store.PublishQueryView
		if err := remoteRequest(cmd, http.MethodGet, "/api/publish-queries/"+url.PathEscape(args[0]), nil, &view, ""); err != nil {
			return err
		}
		return publishQueryOutput(cmd, view)
	}}
	queryShow.Flags().Bool("json", false, "输出JSON")
	confirm := &cobra.Command{Use: "confirm <intent>", Short: "依据外部证据确认原意图", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		filename, _ := cmd.Flags().GetString("decision-file")
		if filename == "" {
			return errors.New("需要私有decision-file")
		}
		data, err := readPublishDecision(filename)
		if err != nil {
			return err
		}
		var input store.ConfirmPublishInput
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || input.IntentID != args[0] {
			return errors.New("发布决定文件无效")
		}
		if decoder.Decode(new(any)) != io.EOF {
			return errors.New("发布决定文件无效")
		}
		var view store.PublishView
		if err = remoteRequest(cmd, http.MethodPost, "/api/publishes/"+url.PathEscape(args[0])+"/confirm", input, &view, ""); err != nil {
			return err
		}
		return publishesOutput(cmd, view, []store.PublishView{view})
	}}
	confirm.Flags().String("decision-file", "", "自有0600普通JSON文件")
	confirm.Flags().Bool("json", false, "输出JSON")
	root.AddCommand(list, show, query, queryShow, confirm)
	return root
}
func publishesOutput(cmd *cobra.Command, value any, items []store.PublishView) error {
	rows := [][]string{}
	for _, v := range items {
		rows = append(rows, []string{v.ID, v.Store, v.AppIdentifier, v.Action, v.Status, strconv.FormatBool(v.ApplicationProtected)})
	}
	return remoteOutput(cmd, value, []string{"INTENT", "STORE", "APPLICATION", "ACTION", "STATUS", "PROTECTED"}, rows)
}
func publishQueryOutput(cmd *cobra.Command, v store.PublishQueryView) error {
	return remoteOutput(cmd, v, []string{"QUERY", "KIND", "STATUS", "OBSERVED", "REASON"}, [][]string{{v.ID, v.Kind, v.Status, v.ObservedLifecycle, v.Reason}})
}
func newProjectAppCommand() *cobra.Command {
	root := &cobra.Command{Use: "app", Short: "绑定项目与实际商店应用", RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
	bind := &cobra.Command{Use: "bind <project>", Short: "登记绑定并请求原节点只读诊断", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		target, _ := cmd.Flags().GetString("store")
		app, _ := cmd.Flags().GetString("app-id")
		node, _ := cmd.Flags().GetString("node")
		env, _ := cmd.Flags().GetString("credentials-env")
		cert, _ := cmd.Flags().GetString("upload-cert-sha256")
		tracks, _ := cmd.Flags().GetStringSlice("track")
		if target == "" || app == "" || node == "" || env == "" {
			return errors.New("需要store、app-id、node和credentials-env")
		}
		input := store.BindApplicationInput{NodeID: node, Store: target, AppIdentifier: app, CredentialRef: "${" + env + "}", UploadCertificateSHA256: cert, AllowedTracks: tracks}
		var out store.ApplicationView
		if err := remoteRequest(cmd, http.MethodPost, "/api/projects/"+url.PathEscape(args[0])+"/applications", input, &out, ""); err != nil {
			return err
		}
		return remoteOutput(cmd, out, []string{"BINDING", "STORE", "APPLICATION", "STATUS"}, [][]string{{out.ID, out.Store, out.AppIdentifier, out.Status}})
	}}
	for _, name := range []string{"store", "app-id", "node", "credentials-env", "upload-cert-sha256"} {
		bind.Flags().String(name, "", "明确绑定设置")
	}
	bind.Flags().StringSlice("track", []string{"internal"}, "允许的Google Play轨道")
	bind.Flags().Bool("json", false, "输出JSON")
	list := &cobra.Command{Use: "ls <project>", Short: "列出安全应用绑定", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var items []store.ApplicationView
		if err := remoteRequest(cmd, http.MethodGet, "/api/projects/"+url.PathEscape(args[0])+"/applications", nil, &items, ""); err != nil {
			return err
		}
		rows := [][]string{}
		for _, v := range items {
			rows = append(rows, []string{v.ID, v.Store, v.AppIdentifier, v.Status})
		}
		return remoteOutput(cmd, items, []string{"BINDING", "STORE", "APPLICATION", "STATUS"}, rows)
	}}
	list.Flags().Bool("json", false, "输出JSON")
	doctor := &cobra.Command{Use: "doctor <binding>", Short: "重新请求指定绑定的实际只读诊断", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var view store.PublishQueryView
		if err := remoteRequest(cmd, http.MethodPost, "/api/applications/"+url.PathEscape(args[0])+"/doctor", nil, &view, ""); err != nil {
			return err
		}
		return publishQueryOutput(cmd, view)
	}}
	doctor.Flags().Bool("json", false, "输出JSON")
	root.AddCommand(bind, list, doctor)
	return root
}

// 读取有界配置复用原no-follow/nonblock/身份与链接数检查，避免FIFO或路径替换。
func readPublishDecision(filename string) ([]byte, error) {
	data, err := config.ReadPrivateJSON(filename, 64<<10)
	if err != nil {
		return nil, errors.New("发布决定需要自有0600普通文件")
	}
	return data, nil
}
