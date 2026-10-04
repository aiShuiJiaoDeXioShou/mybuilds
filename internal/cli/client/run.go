package client

import (
	"encoding/json"
	"errors"
	"strings"

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
		Short: "校验并安全预览流水线",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !dryRun {
				return errors.New("流水线执行尚未实现，请使用 --dry-run 预览")
			}
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
			overrides := make(map[string]string, len(params))
			for _, param := range params {
				key, value, ok := strings.Cut(param, "=")
				if !ok || key == "" {
					return errors.New("--param 必须为非空参数名=value")
				}
				if _, exists := overrides[key]; exists {
					return errors.New("--param 不能重复声明参数")
				}
				overrides[key] = value
			}
			document, err := config.Load(filename)
			if err != nil {
				return err
			}
			plan, err := pipeline.Preview(document, pipeline.PreviewOptions{
				Names: names, All: all, Params: overrides, Step: step,
			})
			if err != nil {
				return err
			}
			output := json.NewEncoder(cmd.OutOrStdout())
			output.SetIndent("", "  ")
			return output.Encode(plan)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "仅校验和预览，不执行任何步骤")
	cmd.Flags().StringVar(&filename, "file", "mybuilds.yml", "流水线配置文件")
	cmd.Flags().StringArrayVar(&builds, "build", nil, "构建名称，多个名称用逗号分隔")
	cmd.Flags().BoolVar(&all, "all", false, "按名称排序预览全部构建")
	cmd.Flags().StringArrayVar(&params, "param", nil, "覆盖参数 key=value，可重复使用")
	cmd.Flags().StringVar(&step, "step", "", "预览单个构建中的指定步骤")
	return cmd
}
