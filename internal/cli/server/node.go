package server

import (
	"errors"
	"github.com/spf13/cobra"
	"mybuilds/internal/store"
	"strconv"
)

func localNodeRows(items []store.NodeView) [][]string {
	rows := make([][]string, 0, len(items))
	for _, v := range items {
		rows = append(rows, []string{v.Name, v.State, v.OS, v.Arch, strconv.Itoa(v.EffectiveCapacity), strconv.Itoa(v.Running), strconv.FormatBool(v.Healthy), strconv.FormatBool(v.Quarantined)})
	}
	return rows
}
func newNodeCommand() *cobra.Command {
	node := &cobra.Command{Use: "node", Short: "管理独立构建节点"}
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
		return withStore(cmd, func(db *store.Store) error {
			result, err := db.CreateNode(cmd.Context(), localAdmin, store.NodeInput{Name: args[0], Labels: labels, Capacity: capacity})
			if err != nil {
				return err
			}
			return managementOutput(cmd, result, []string{"NAME", "STATE", "TOKEN"}, [][]string{{result.Node.Name, result.Node.State, result.Token}})
		})
	}}
	create.Flags().Int("capacity", 1, "管理员节点容量，1–32")
	create.Flags().StringSlice("labels", nil, "管理员标签，逗号分隔")
	create.Flags().Bool("json", false, "输出JSON")
	list := &cobra.Command{Use: "ls", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		page, err := managementPage(cmd)
		if err != nil {
			return err
		}
		return withStore(cmd, func(db *store.Store) error {
			items, err := db.ListNodes(cmd.Context(), localAdmin, store.NodeFilter{Page: page})
			if err != nil {
				return err
			}
			return managementOutput(cmd, struct {
				Items  []store.NodeView `json:"items"`
				Limit  int              `json:"limit"`
				Offset int              `json:"offset"`
			}{items, page.Limit, page.Offset}, columns, localNodeRows(items))
		})
	}}
	pageFlags(list)
	show := &cobra.Command{Use: "show <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return withStore(cmd, func(db *store.Store) error {
			item, err := db.GetNode(cmd.Context(), localAdmin, args[0])
			if err != nil {
				return err
			}
			return managementOutput(cmd, item, columns, localNodeRows([]store.NodeView{item}))
		})
	}}
	show.Flags().Bool("json", false, "输出JSON")
	node.AddCommand(create, list, show)
	for _, action := range []string{"drain", "enable", "disable", "rm"} {
		action := action
		node.AddCommand(&cobra.Command{Use: action + " <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(cmd, func(db *store.Store) error {
				if action == "rm" {
					return db.DeleteNode(cmd.Context(), localAdmin, args[0])
				}
				return db.SetNodeState(cmd.Context(), localAdmin, args[0], map[string]string{"drain": "draining", "enable": "enabled", "disable": "disabled"}[action])
			})
		}})
	}
	token := &cobra.Command{Use: "token", Short: "撤销或更换节点凭据", Args: func(_ *cobra.Command, args []string) error {
		if len(args) > 0 {
			return errors.New("节点凭据子命令无效")
		}
		return nil
	}, RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
	rotate := &cobra.Command{Use: "rotate <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return withStore(cmd, func(db *store.Store) error {
			result, err := db.RotateNodeToken(cmd.Context(), localAdmin, args[0])
			if err != nil {
				return err
			}
			return managementOutput(cmd, result, []string{"NAME", "TOKEN"}, [][]string{{result.Node.Name, result.Token}})
		})
	}}
	rotate.Flags().Bool("json", false, "输出JSON")
	revoke := &cobra.Command{Use: "revoke <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return withStore(cmd, func(db *store.Store) error { return db.RevokeNodeToken(cmd.Context(), localAdmin, args[0]) })
	}}
	token.AddCommand(rotate, revoke)
	node.AddCommand(token)
	return node
}
