package client

import (
	"errors"
	"github.com/spf13/cobra"
	"mybuilds/internal/store"
	"net/http"
	"net/url"
	"strconv"
)

func newRemoteGroupCommand() *cobra.Command {
	group := &cobra.Command{Use: "group", Short: "远程管理项目组"}
	create := &cobra.Command{Use: "create <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var result store.Group
		if err := remoteRequest(cmd, http.MethodPost, "/api/groups", struct {
			Name string `json:"name"`
		}{args[0]}, &result, ""); err != nil {
			return err
		}
		return remoteOutput(cmd, result, []string{"ID", "NAME"}, [][]string{{result.ID, result.Name}})
	}}
	create.Flags().Bool("json", false, "输出JSON")
	list := &cobra.Command{Use: "ls", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		limit, offset, err := remotePage(cmd)
		if err != nil {
			return err
		}
		var result struct {
			Items  []store.Group `json:"items"`
			Limit  int           `json:"limit"`
			Offset int           `json:"offset"`
		}
		path := "/api/groups?limit=" + strconv.Itoa(limit) + "&offset=" + strconv.Itoa(offset)
		if err := remoteRequest(cmd, http.MethodGet, path, nil, &result, ""); err != nil {
			return err
		}
		rows := make([][]string, 0, len(result.Items))
		for _, v := range result.Items {
			rows = append(rows, []string{v.ID, v.Name})
		}
		return remoteOutput(cmd, result, []string{"ID", "NAME"}, rows)
	}}
	remotePageFlags(list)
	rename := &cobra.Command{Use: "rename <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		if name == "" {
			return errors.New("需要新组名")
		}
		var result store.Group
		if err := remoteRequest(cmd, http.MethodPatch, "/api/groups/"+url.PathEscape(args[0]), struct {
			Name string `json:"name"`
		}{name}, &result, ""); err != nil {
			return err
		}
		return remoteOutput(cmd, result, []string{"ID", "NAME"}, [][]string{{result.ID, result.Name}})
	}}
	rename.Flags().String("name", "", "新组名")
	rename.Flags().Bool("json", false, "输出JSON")
	remove := &cobra.Command{Use: "rm <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return remoteRequest(cmd, http.MethodDelete, "/api/groups/"+url.PathEscape(args[0]), nil, nil, "")
	}}
	group.AddCommand(create, list, rename, remove)
	return group
}
