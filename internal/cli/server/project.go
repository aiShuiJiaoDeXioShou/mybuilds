package server

import (
	"errors"
	"github.com/spf13/cobra"
	"mybuilds/internal/config"
	control "mybuilds/internal/server"
	"mybuilds/internal/store"
	"strconv"
)

func projectInputFlags(cmd *cobra.Command) {
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
func localProjectInput(cmd *cobra.Command, name string) (store.ProjectInput, error) {
	for _, flag := range []string{"hook", "poll", "schedule"} {
		if cmd.Flags().Changed(flag) {
			return store.ProjectInput{}, errors.New("项目绑定方案与自动触发尚未支持")
		}
	}
	if cmd.Flags().Changed("file") && cmd.Flags().Changed("settings") {
		return store.ProjectInput{}, errors.New("file与settings不能同时指定")
	}
	if (cmd.Flags().Changed("framework") || cmd.Flags().Changed("platform")) && (cmd.Flags().Changed("file") || cmd.Flags().Changed("settings")) {
		return store.ProjectInput{}, errors.New("框架平台不能与file/settings混用")
	}
	if cmd.Flags().Changed("framework") && !cmd.Flags().Changed("platform") {
		return store.ProjectInput{}, errors.New("framework需要platform")
	}
	var input store.ProjectInput
	input.Name = name
	input.Repository, _ = cmd.Flags().GetString("repo")
	input.Provider, _ = cmd.Flags().GetString("provider")
	input.Group, _ = cmd.Flags().GetString("group")
	input.Branches, _ = cmd.Flags().GetStringSlice("branches")
	input.AllowedNodes, _ = cmd.Flags().GetStringSlice("nodes")
	input.DefaultNode, _ = cmd.Flags().GetString("default-node")
	input.BuildNumberStart, _ = cmd.Flags().GetInt64("build-number-start")
	if input.Repository == "" || len(input.AllowedNodes) == 0 || input.BuildNumberStart < 1 {
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
func localProjectOutput(cmd *cobra.Command, value store.Project) error {
	view := control.ProjectSummary(value)
	return managementOutput(cmd, view, []string{"ID", "NAME", "GROUP", "NEXT"}, [][]string{{view.ID, view.Name, view.Group, strconv.FormatInt(view.NextNumber, 10)}})
}
func newProjectCommand() *cobra.Command {
	project := &cobra.Command{Use: "project", Short: "管理可信项目"}
	add := &cobra.Command{Use: "add <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		input, err := localProjectInput(cmd, args[0])
		if err != nil {
			return err
		}
		return withStore(cmd, func(db *store.Store) error {
			value, err := db.CreateProject(cmd.Context(), localAdmin, input)
			if err != nil {
				return err
			}
			return localProjectOutput(cmd, value)
		})
	}}
	projectInputFlags(add)
	add.Flags().Bool("json", false, "输出JSON")
	set := &cobra.Command{Use: "set <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		filename, _ := cmd.Flags().GetString("settings")
		if filename == "" {
			return errors.New("需要settings文件")
		}
		settings, err := config.LoadProjectSettings(filename)
		if err != nil {
			return err
		}
		return withStore(cmd, func(db *store.Store) error {
			value, err := db.SetProjectSettings(cmd.Context(), localAdmin, args[0], settings)
			if err != nil {
				return err
			}
			return localProjectOutput(cmd, value)
		})
	}}
	set.Flags().String("settings", "", "本地settings文件")
	set.Flags().Bool("json", false, "输出JSON")
	list := &cobra.Command{Use: "ls", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		page, err := managementPage(cmd)
		if err != nil {
			return err
		}
		group, _ := cmd.Flags().GetString("group")
		return withStore(cmd, func(db *store.Store) error {
			values, err := db.ListProjects(cmd.Context(), store.ProjectFilter{Group: group, Page: page})
			if err != nil {
				return err
			}
			items := make([]control.ProjectView, 0, len(values))
			rows := make([][]string, 0, len(values))
			for _, value := range values {
				v := control.ProjectSummary(value)
				items = append(items, v)
				rows = append(rows, []string{v.ID, v.Name, v.Group, strconv.FormatInt(v.NextNumber, 10)})
			}
			return managementOutput(cmd, struct {
				Items  []control.ProjectView `json:"items"`
				Limit  int                   `json:"limit"`
				Offset int                   `json:"offset"`
			}{items, page.Limit, page.Offset}, []string{"ID", "NAME", "GROUP", "NEXT"}, rows)
		})
	}}
	pageFlags(list)
	list.Flags().String("group", "", "按组过滤")
	move := &cobra.Command{Use: "move <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		group, _ := cmd.Flags().GetString("group")
		if group == "" {
			return errors.New("需要目标组")
		}
		return withStore(cmd, func(db *store.Store) error {
			value, err := db.MoveProject(cmd.Context(), localAdmin, args[0], group)
			if err != nil {
				return err
			}
			return localProjectOutput(cmd, value)
		})
	}}
	move.Flags().String("group", "", "目标项目组")
	move.Flags().Bool("json", false, "输出JSON")
	remove := &cobra.Command{Use: "rm <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return withStore(cmd, func(db *store.Store) error { return db.DeleteProject(cmd.Context(), localAdmin, args[0]) })
	}}
	project.AddCommand(add, set, list, move, remove)
	return project
}
