package client

import (
	"encoding/json"
	"errors"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"mybuilds/internal/config"
	"mybuilds/internal/pipeline"
)

func newRunCommand() *cobra.Command {
	var dryRun, all bool
	var filename, step string
	var builds, params []string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "执行或安全预览本地流水线",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("build") && cmd.Flags().Changed("all") {
				return errors.New("--build 与 --all 不能同时使用")
			}
			if cmd.Flags().Changed("step") && step == "" {
				return errors.New("--step 需要步骤名称")
			}
			var names []string
			seen := make(map[string]bool)
			for _, list := range builds {
				for _, name := range strings.Split(list, ",") {
					if name == "" || seen[name] {
						return errors.New("--build 不能包含空项或重复名称")
					}
					seen[name] = true
					names = append(names, name)
				}
			}
			overrides, scoped, err := triggerParameters(params, "", "")
			if err != nil {
				return err
			}
			document, err := config.Load(filename)
			if err != nil {
				return err
			}
			options := pipeline.PreviewOptions{Names: names, All: all, Params: overrides, BuildParams: scoped, Step: step}
			output := json.NewEncoder(cmd.OutOrStdout())
			output.SetIndent("", "  ")
			if dryRun {
				plan, err := pipeline.Preview(document, options)
				if err != nil {
					return err
				}
				if err := output.Encode(plan); err != nil {
					return errors.New("写入预览结果失败")
				}
				return nil
			}
			workspace, err := os.Getwd()
			if err != nil {
				return errors.New("读取当前工作目录失败")
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			result, runErr := pipeline.Run(ctx, document, pipeline.RunOptions{
				PreviewOptions: options, Workspace: workspace, Output: cmd.ErrOrStderr(),
			})
			if result != nil {
				if err := output.Encode(result); err != nil {
					return errors.New("写入运行结果失败")
				}
			}
			return runErr
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "仅校验和预览，不执行任何步骤")
	cmd.Flags().StringVar(&filename, "file", "mybuilds.yml", "流水线配置文件")
	cmd.Flags().StringArrayVar(&builds, "build", nil, "构建名称，多个名称用逗号分隔")
	cmd.Flags().BoolVar(&all, "all", false, "按名称排序选择全部构建")
	cmd.Flags().StringArrayVar(&params, "param", nil, "覆盖参数 key=value 或 build:key=value，可重复使用")
	cmd.Flags().StringVar(&step, "step", "", "选择单个构建中的指定步骤")
	return cmd
}
