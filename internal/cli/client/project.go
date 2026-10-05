package client

import (
	"errors"
	"github.com/spf13/cobra"
	"mybuilds/internal/config"
	"mybuilds/internal/server"
	"net/http"
	"net/url"
	"strconv"
)

func remoteProjectFlags(cmd *cobra.Command) {
	cmd.Flags().String("repo", "", "可信Git仓库")
	cmd.Flags().String("provider", "generic", "仓库提供方")
	cmd.Flags().String("group", "default", "项目组")
	cmd.Flags().StringSlice("branches", []string{"main"}, "允许分支")
	cmd.Flags().StringSlice("nodes", nil, "授权节点名")
	cmd.Flags().String("default-node", "", "默认节点")
	cmd.Flags().Int64("build-number-start", 1, "首次构建编号")
	cmd.Flags().String("settings", "", "本地settings文件")
	cmd.Flags().String("file", "", "仓库相对流水线文件")
	for _, name := range []string{"poll", "schedule"} {
		cmd.Flags().String(name, "", "当前未实现")
	}
	cmd.Flags().String("framework", "", "构建框架native/flutter")
	cmd.Flags().String("platform", "", "android/ios或android,ios")
	cmd.Flags().Bool("hook", false, "当前未实现")
}
func remoteProjectInput(cmd *cobra.Command, name string) (server.ProjectRequest, error) {
	for _, flag := range []string{"hook", "poll", "schedule"} {
		if cmd.Flags().Changed(flag) {
			return server.ProjectRequest{}, errors.New("项目绑定方案与自动触发尚未支持")
		}
	}
	if cmd.Flags().Changed("file") && cmd.Flags().Changed("settings") {
		return server.ProjectRequest{}, errors.New("file与settings不能同时指定")
	}
	if (cmd.Flags().Changed("framework") || cmd.Flags().Changed("platform")) && (cmd.Flags().Changed("file") || cmd.Flags().Changed("settings")) {
		return server.ProjectRequest{}, errors.New("框架平台不能与file/settings混用")
	}
	if cmd.Flags().Changed("framework") && !cmd.Flags().Changed("platform") {
		return server.ProjectRequest{}, errors.New("framework需要platform")
	}
	var input server.ProjectRequest
	input.Name = name
	input.Repository, _ = cmd.Flags().GetString("repo")
	input.Provider, _ = cmd.Flags().GetString("provider")
	input.Group, _ = cmd.Flags().GetString("group")
	input.Branches, _ = cmd.Flags().GetStringSlice("branches")
	input.Nodes, _ = cmd.Flags().GetStringSlice("nodes")
	input.DefaultNode, _ = cmd.Flags().GetString("default-node")
	input.BuildNumberStart, _ = cmd.Flags().GetInt64("build-number-start")
	if input.Repository == "" || len(input.Nodes) == 0 || input.BuildNumberStart < 1 {
		return input, errors.New("需要repo、nodes与正起始编号")
	}
	if cmd.Flags().Changed("settings") {
		filename, _ := cmd.Flags().GetString("settings")
		if filename == "" {
			return input, errors.New("需要settings文件")
		}
		var err error
		input.Settings, err = config.LoadProjectSettings(filename)
		if err != nil {
			return input, err
		}
	}
	if cmd.Flags().Changed("file") {
		file, _ := cmd.Flags().GetString("file")
		if file == "" {
			return input, errors.New("需要仓库相对file")
		}
		input.Settings = config.ProjectSettings{Pipeline: &config.PipelineSettings{Source: "repo", File: file}}
	}
	if cmd.Flags().Changed("platform") {
		framework, _ := cmd.Flags().GetString("framework")
		platform, _ := cmd.Flags().GetString("platform")
		if cmd.Flags().Changed("framework") && framework == "" {
			return input, errors.New("framework不能为空")
		}
		binding, err := config.BindProfiles(framework, platform)
		if err != nil {
			return input, err
		}
		input.Settings.Pipeline = binding
	}
	return input, config.ValidateProjectSettings(input.Settings)
}
func projectRow(v server.ProjectView) []string {
	return []string{v.ID, v.Name, v.Group, strconv.FormatInt(v.NextNumber, 10)}
}
func newRemoteProjectCommand() *cobra.Command {
	project := &cobra.Command{Use: "project", Short: "远程管理可信项目"}
	init := &cobra.Command{Use: "init <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		input, err := remoteProjectInput(cmd, args[0])
		if err != nil {
			return err
		}
		var result server.ProjectView
		if err := remoteRequest(cmd, http.MethodPost, "/api/projects", input, &result, ""); err != nil {
			return err
		}
		return remoteOutput(cmd, result, []string{"ID", "NAME", "GROUP", "NEXT"}, [][]string{projectRow(result)})
	}}
	remoteProjectFlags(init)
	init.Flags().Bool("json", false, "输出JSON")
	set := &cobra.Command{Use: "set <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		filename, _ := cmd.Flags().GetString("settings")
		if filename == "" {
			return errors.New("需要settings文件")
		}
		settings, err := config.LoadProjectSettings(filename)
		if err != nil {
			return err
		}
		var result server.ProjectView
		if err := remoteRequest(cmd, http.MethodPatch, "/api/projects/"+url.PathEscape(args[0]), struct {
			Settings config.ProjectSettings `json:"settings"`
		}{settings}, &result, ""); err != nil {
			return err
		}
		return remoteOutput(cmd, result, []string{"ID", "NAME", "GROUP", "NEXT"}, [][]string{projectRow(result)})
	}}
	set.Flags().String("settings", "", "本地settings文件")
	set.Flags().Bool("json", false, "输出JSON")
	list := &cobra.Command{Use: "ls", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		limit, offset, err := remotePage(cmd)
		if err != nil {
			return err
		}
		query := url.Values{"limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}}
		group, _ := cmd.Flags().GetString("group")
		if group != "" {
			query.Set("group", group)
		}
		var result struct {
			Items  []server.ProjectView `json:"items"`
			Limit  int                  `json:"limit"`
			Offset int                  `json:"offset"`
		}
		if err := remoteRequest(cmd, http.MethodGet, "/api/projects?"+query.Encode(), nil, &result, ""); err != nil {
			return err
		}
		rows := make([][]string, 0, len(result.Items))
		for _, v := range result.Items {
			rows = append(rows, projectRow(v))
		}
		return remoteOutput(cmd, result, []string{"ID", "NAME", "GROUP", "NEXT"}, rows)
	}}
	remotePageFlags(list)
	list.Flags().String("group", "", "按组过滤")
	move := &cobra.Command{Use: "move <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		group, _ := cmd.Flags().GetString("group")
		if group == "" {
			return errors.New("需要目标组")
		}
		var result server.ProjectView
		if err := remoteRequest(cmd, http.MethodPatch, "/api/projects/"+url.PathEscape(args[0]), struct {
			Group string `json:"group"`
		}{group}, &result, ""); err != nil {
			return err
		}
		return remoteOutput(cmd, result, []string{"ID", "NAME", "GROUP", "NEXT"}, [][]string{projectRow(result)})
	}}
	move.Flags().String("group", "", "目标项目组")
	move.Flags().Bool("json", false, "输出JSON")
	remove := &cobra.Command{Use: "rm <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return remoteRequest(cmd, http.MethodDelete, "/api/projects/"+url.PathEscape(args[0]), nil, nil, "")
	}}
	project.AddCommand(newProjectAppCommand(), init, set, list, move, remove)
	return project
}
