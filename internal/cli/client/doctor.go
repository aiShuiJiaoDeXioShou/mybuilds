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
	"regexp"
	"slices"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"mybuilds/internal/mobile"
)

func newDoctorCommand() *cobra.Command {
	var options mobile.AndroidDoctorOptions
	var framework, platform, p12, profile, passwordEnv, bundleID, exportMethod string
	var asJSON bool
	var remoteServer bool
	var remoteNode string
	var publishing publishDoctorOptions
	cmd := &cobra.Command{
		Use: "doctor", Short: "检查本机移动端构建环境", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("target") {
				for _, name := range []string{"server", "node", "framework", "platform", "working-dir", "gradle-wrapper", "keystore", "key-alias", "store-password-env", "key-password-env", "p12", "profile", "password-env", "bundle-id", "export-method"} {
					if cmd.Flags().Changed(name) {
						return errors.New("发布诊断不能混用构建或远程检查选项")
					}
				}
				return publishDoctor(cmd, publishing, asJSON)
			}
			for _, name := range []string{"agent-config", "app-id", "credentials-env", "upload-cert-sha256"} {
				if cmd.Flags().Changed(name) {
					return errors.New("发布诊断选项需要显式target")
				}
			}
			if cmd.Flags().Changed("node") && remoteNode == "" {
				return errors.New("远程节点名称无效")
			}
			if remoteServer || remoteNode != "" {
				if remoteServer && remoteNode != "" {
					return errors.New("远程检查目标互斥")
				}
				for _, name := range []string{"framework", "platform", "working-dir", "gradle-wrapper", "keystore", "key-alias", "store-password-env", "key-password-env", "p12", "profile", "password-env", "bundle-id", "export-method"} {
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
			platforms := strings.Split(platform, ",")
			if framework != "native" && framework != "flutter" {
				return errors.New("指定的框架检查尚未支持")
			}
			seen := map[string]bool{}
			for _, p := range platforms {
				if (p != "android" && p != "ios") || seen[p] {
					return errors.New("平台组合无效")
				}
				seen[p] = true
			}
			if framework == "native" && platform != "android" && platform != "ios" {
				return errors.New("指定的平台检查尚未支持")
			}
			if !slices.Contains(platforms, "android") {
				for _, name := range []string{"gradle-wrapper", "keystore", "key-alias", "store-password-env", "key-password-env"} {
					if cmd.Flags().Changed(name) {
						return errors.New("Android选项不能用于未选平台")
					}
				}
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			checks := []mobile.DoctorCheck{}
			if framework == "flutter" {
				checks = mobile.FlutterDoctor(ctx, mobile.FlutterDoctorOptions{Workspace: options.Workspace, Platforms: platforms})
			}
			safe := ctx.Err() == nil
			for _, check := range checks {
				if check.Reason == "cleanup_error" {
					safe = false
				}
			}
			if !slices.Contains(platforms, "ios") {
				for _, name := range []string{"p12", "profile", "password-env", "bundle-id", "export-method"} {
					if cmd.Flags().Changed(name) {
						return errors.New("平台选项不可混用")
					}
				}
			}
			if safe && slices.Contains(platforms, "android") {
				checks = append(checks, mobile.AndroidDoctor(ctx, options)...)
			}
			if safe && slices.Contains(platforms, "ios") {
				ios := mobile.IOSDoctorOptions{Workspace: options.Workspace}
				declared := false
				for _, name := range []string{"p12", "profile", "password-env", "bundle-id", "export-method"} {
					declared = declared || cmd.Flags().Changed(name)
				}
				if declared {
					if p12 == "" || profile == "" || bundleID == "" || exportMethod == "" || !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(passwordEnv) {
						return errors.New("ios_signing_options_invalid")
					}
					password, ok := os.LookupEnv(passwordEnv)
					if !ok {
						return errors.New("ios_signing_environment_unavailable")
					}
					ios.Signing = &mobile.IOSSigningOptions{Workspace: options.Workspace, P12File: p12, ProfileFile: profile, Password: password, BundleID: bundleID, ExportMethod: exportMethod}
				}
				checks = append(checks, mobile.IOSDoctor(ctx, ios)...)
			}
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
	cmd.Flags().StringVar(&publishing.Target, "target", "", "显式诊断google-play或app-store发布环境")
	cmd.Flags().StringVar(&publishing.AgentConfig, "agent-config", "", "发布工具所在Agent配置，不加载身份token")
	cmd.Flags().StringVar(&publishing.AppID, "app-id", "", "明确的包名或bundle ID")
	cmd.Flags().StringVar(&publishing.CredentialsEnv, "credentials-env", "", "发布材料路径的环境变量名")
	cmd.Flags().StringVar(&publishing.Certificate, "upload-cert-sha256", "", "Google Play上传证书摘要")
	cmd.Flags().StringVar(&remoteNode, "node", "", "检查指定节点最近实际报告")
	cmd.Flags().StringVar(&framework, "framework", "native", "构建框架（native/flutter）")
	cmd.Flags().StringVar(&platform, "platform", "android", "检查目标平台")
	cmd.Flags().BoolVar(&asJSON, "json", false, "输出 JSON 检查记录")
	cmd.Flags().StringVar(&options.Workspace, "working-dir", "", "工程目录，默认当前目录")
	cmd.Flags().StringVar(&options.GradleWrapper, "gradle-wrapper", "gradlew", "工程内的 Gradle wrapper 相对路径")
	cmd.Flags().StringVar(&options.Keystore, "keystore", "", "显式检查的 keystore 路径")
	cmd.Flags().StringVar(&options.KeyAlias, "key-alias", "", "显式私钥 alias")
	cmd.Flags().StringVar(&options.StorePasswordEnv, "store-password-env", "", "keystore 密码的环境变量名")
	cmd.Flags().StringVar(&options.KeyPasswordEnv, "key-password-env", "", "私钥密码的环境变量名")
	cmd.Flags().StringVar(&p12, "p12", "", "明确的P12文件")
	cmd.Flags().StringVar(&profile, "profile", "", "明确的provisioning profile文件")
	cmd.Flags().StringVar(&passwordEnv, "password-env", "", "P12密码的环境变量名")
	cmd.Flags().StringVar(&bundleID, "bundle-id", "", "明确的Bundle ID")
	cmd.Flags().StringVar(&exportMethod, "export-method", "", "明确的导出方法")
	return cmd
}
