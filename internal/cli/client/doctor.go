package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"mybuilds/internal/mobile"
)

func newDoctorCommand() *cobra.Command {
	var options mobile.AndroidDoctorOptions
	var platform string
	var asJSON bool
	cmd := &cobra.Command{
		Use: "doctor", Short: "检查本机移动端构建环境", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if platform != "android" {
				return errors.New("指定的平台检查尚未支持")
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			checks := mobile.AndroidDoctor(ctx, options)
			if asJSON {
				output := json.NewEncoder(cmd.OutOrStdout())
				output.SetIndent("", "  ")
				if err := output.Encode(checks); err != nil {
					return errors.New("写入检查结果失败")
				}
			} else {
				for _, check := range checks {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: %s %s %s\n", check.Name, check.Status, check.Version, check.Reason); err != nil {
						return errors.New("写入检查结果失败")
					}
				}
			}
			for _, check := range checks {
				if check.Status == "failed" {
					return errors.New("本机检查未通过")
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&platform, "platform", "android", "检查目标平台")
	cmd.Flags().BoolVar(&asJSON, "json", false, "输出 JSON 检查记录")
	cmd.Flags().StringVar(&options.Workspace, "working-dir", "", "工程目录，默认当前目录")
	cmd.Flags().StringVar(&options.GradleWrapper, "gradle-wrapper", "gradlew", "工程内的 Gradle wrapper 相对路径")
	cmd.Flags().StringVar(&options.Keystore, "keystore", "", "显式检查的 keystore 路径")
	cmd.Flags().StringVar(&options.KeyAlias, "key-alias", "", "显式私钥 alias")
	cmd.Flags().StringVar(&options.StorePasswordEnv, "store-password-env", "", "keystore 密码的环境变量名")
	cmd.Flags().StringVar(&options.KeyPasswordEnv, "key-password-env", "", "私钥密码的环境变量名")
	return cmd
}
