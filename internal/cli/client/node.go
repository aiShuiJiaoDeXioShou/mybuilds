package client

import (
	"net/http"
	"net/url"
	"strconv"

	"errors"
	"github.com/spf13/cobra"
	"mybuilds/internal/server"
	"mybuilds/internal/store"
)

func remoteNodeRows(items []store.NodeView) [][]string {
	rows := make([][]string, 0, len(items))
	for _, v := range items {
		rows = append(rows, []string{v.Name, v.State, v.OS, v.Arch, strconv.Itoa(v.EffectiveCapacity), strconv.Itoa(v.Running), strconv.FormatBool(v.Healthy), strconv.FormatBool(v.Quarantined)})
	}
	return rows
}
func newRemoteNodeCommand() *cobra.Command {
	node := &cobra.Command{Use: "node", Short: "远程管理独立构建节点"}
	node.Args = func(_ *cobra.Command, args []string) error {
		if len(args) > 0 {
			return errors.New("节点子命令无效")
		}
		return nil
	}
	node.RunE = func(cmd *cobra.Command, _ []string) error { return cmd.Help() }
	columns := []string{"NAME", "STATE", "OS", "ARCH", "CAPACITY", "RUNNING", "HEALTHY", "QUARANTINED"}
	create := &cobra.Command{Use: "create <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		capacity, _ := cmd.Flags().GetInt("capacity")
		labels, _ := cmd.Flags().GetStringSlice("labels")
		if labels == nil {
			labels = []string{}
		}
		var result store.NodeCreated
		if err := remoteRequest(cmd, http.MethodPost, "/api/nodes", server.NodeRequest{Name: args[0], Labels: labels, Capacity: capacity}, &result, ""); err != nil {
			return err
		}
		return remoteOutput(cmd, result, []string{"NAME", "STATE", "TOKEN"}, [][]string{{result.Node.Name, result.Node.State, result.Token}})
	}}
	create.Flags().Int("capacity", 1, "管理员节点容量，1–32")
	create.Flags().StringSlice("labels", nil, "管理员标签，逗号分隔")
	create.Flags().Bool("json", false, "输出JSON")
	list := &cobra.Command{Use: "ls", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		limit, offset, err := remotePage(cmd)
		if err != nil {
			return err
		}
		var result struct {
			Items  []store.NodeView `json:"items"`
			Limit  int              `json:"limit"`
			Offset int              `json:"offset"`
		}
		if err := remoteRequest(cmd, http.MethodGet, "/api/nodes?limit="+strconv.Itoa(limit)+"&offset="+strconv.Itoa(offset), nil, &result, ""); err != nil {
			return err
		}
		return remoteOutput(cmd, result, columns, remoteNodeRows(result.Items))
	}}
	remotePageFlags(list)
	show := &cobra.Command{Use: "show <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var result store.NodeView
		if err := remoteRequest(cmd, http.MethodGet, "/api/nodes/"+url.PathEscape(args[0]), nil, &result, ""); err != nil {
			return err
		}
		return remoteOutput(cmd, result, columns, remoteNodeRows([]store.NodeView{result}))
	}}
	show.Flags().Bool("json", false, "输出JSON")
	node.AddCommand(create, list, show)
	for _, action := range []string{"drain", "enable", "disable", "rm"} {
		action := action
		node.AddCommand(&cobra.Command{Use: action + " <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			method, path := http.MethodPost, "/api/nodes/"+url.PathEscape(args[0])+"/"+action
			if action == "rm" {
				method, path = http.MethodDelete, "/api/nodes/"+url.PathEscape(args[0])
			}
			return remoteRequest(cmd, method, path, nil, nil, "")
		}})
	}
	token := &cobra.Command{Use: "token", Short: "撤销或更换节点凭据", Args: func(_ *cobra.Command, args []string) error {
		if len(args) > 0 {
			return errors.New("节点凭据子命令无效")
		}
		return nil
	}, RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
	rotate := &cobra.Command{Use: "rotate <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var result store.NodeCreated
		if err := remoteRequest(cmd, http.MethodPost, "/api/nodes/"+url.PathEscape(args[0])+"/token/rotate", nil, &result, ""); err != nil {
			return err
		}
		return remoteOutput(cmd, result, []string{"NAME", "TOKEN"}, [][]string{{result.Node.Name, result.Token}})
	}}
	rotate.Flags().Bool("json", false, "输出JSON")
	revoke := &cobra.Command{Use: "revoke <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return remoteRequest(cmd, http.MethodPost, "/api/nodes/"+url.PathEscape(args[0])+"/token/revoke", nil, nil, "")
	}}
	token.AddCommand(rotate, revoke)
	node.AddCommand(token)
	return node
}
