package client

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"mybuilds/internal/config"
	"mybuilds/internal/mobile"
)

const defaultPipeline = `version: 1
steps:
  - kind: run
    name: hello
    run: echo "请编辑 mybuilds.yml 配置构建步骤"
`

func newInitCommand() *cobra.Command {
	var template, framework, platform string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "在当前目录创建流水线配置",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			hasTemplate := cmd.Flags().Changed("template")
			hasPlatform := cmd.Flags().Changed("framework") || cmd.Flags().Changed("platform")
			if hasTemplate && hasPlatform {
				return errors.New("本地模板不能与 framework/platform 同时使用")
			}
			data := []byte(defaultPipeline)
			if hasPlatform {
				if cmd.Flags().Changed("framework") && framework == "" {
					return errors.New("framework 不能为空")
				}
				if framework == "" {
					framework = "native"
				}
				if framework != "native" || (platform != "android" && platform != "ios") {
					return errors.New("指定的框架或平台模板尚未支持")
				}
				if platform == "ios" {
					data = mobile.IOSTemplate()
				} else {
					data = mobile.AndroidTemplate()
				}
			}
			if hasTemplate {
				file, err := os.Open(template)
				if err != nil {
					return errors.New("读取本地模板失败")
				}
				data, err = io.ReadAll(io.LimitReader(file, config.MaxConfigBytes+1))
				closeErr := file.Close()
				if err != nil || closeErr != nil {
					return errors.New("读取本地模板失败")
				}
				if _, err := config.Parse(data); err != nil {
					return fmt.Errorf("本地模板无效：%w", err)
				}
			}
			if err := createConfig("mybuilds.yml", data); err != nil {
				return err
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "已创建 mybuilds.yml")
			return err
		},
	}
	cmd.Flags().StringVar(&template, "template", "", "使用经校验的本地 YAML 模板")
	cmd.Flags().StringVar(&framework, "framework", "", "构建框架（指定平台时默认 native）")
	cmd.Flags().StringVar(&platform, "platform", "", "目标平台")
	return cmd
}

func createConfig(filename string, data []byte) error {
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return errors.New("mybuilds.yml 已存在，拒绝覆盖")
		}
		return errors.New("创建 mybuilds.yml 失败")
	}
	owned, statErr := file.Stat()
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if statErr != nil || writeErr != nil || closeErr != nil {
		// 仅清理本次创建的文件，避免删除已被其他进程替换的目标。
		if current, err := os.Lstat(filename); err == nil && owned != nil && os.SameFile(owned, current) {
			if err := os.Remove(filename); err != nil {
				return errors.New("写入配置失败，清理未完成的 mybuilds.yml 失败")
			}
		}
		return errors.New("写入 mybuilds.yml 失败")
	}
	return nil
}
