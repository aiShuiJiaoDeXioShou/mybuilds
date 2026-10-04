package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"mybuilds/internal/server"
	"net/http"
	"net/url"
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
	var remoteServer bool
	var remoteNode string
	cmd := &cobra.Command{
		Use: "doctor", Short: "检查本机移动端构建环境", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("node") && remoteNode == "" {
				return errors.New("远程节点名称无效")
			}
			if remoteServer || remoteNode != "" {
				if remoteServer && remoteNode != "" {
					return errors.New("远程检查目标互斥")
				}
				for _, name := range []string{"platform", "working-dir", "gradle-wrapper", "keystore", "key-alias", "store-password-env", "key-password-env"} {
					if cmd.Flags().Changed(name) {
						return errors.New("远程检查不能混用本地工程选项")
					}
				}
				if remoteServer {
					var result server.ControlDoctorDTO
					if err := remoteRequest(cmd, http.MethodGet, "/api/doctor", nil, &result, ""); err != nil {
						return err
					}
					return remoteOutput(cmd, result, []string{"STATUS", "NODES", "HEALTHY_NODES"}, [][]string{{result.Status, fmt.Sprint(result.Control.Nodes), fmt.Sprint(result.Control.HealthyNodes)}})
				}
				var result server.NodeDoctorDTO
				if err := remoteRequest(cmd, http.MethodGet, "/api/nodes/"+url.PathEscape(remoteNode)+"/doctor", nil, &result, ""); err != nil {
					return err
				}
				rows := [][]string{{result.Node.Name, result.Status, result.Reason}}
				for _, tool := range result.Node.Tools {
					rows = append(rows, []string{tool.Name, tool.Status, tool.Version + " " + tool.Reason})
				}
				if err := remoteOutput(cmd, result, []string{"CHECK", "STATUS", "DETAIL"}, rows); err != nil {
					return err
				}
				if result.Status == "failed" {
					return errors.New("远程检查未通过")
				}
				return nil
			}
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
	cmd.Flags().BoolVar(&remoteServer, "server", false, "检查控制端")
	cmd.Flags().StringVar(&remoteNode, "node", "", "检查指定节点最近实际报告")
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
