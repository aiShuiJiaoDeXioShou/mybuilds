// Package agent提供独立节点入口，管理身份只在serve读取。
package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	node "mybuilds/internal/agent"
	"mybuilds/internal/version"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "mybuilds-agent", Short: "独立构建节点", SilenceUsage: true}
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, _ error) error { return errors.New("命令选项不合法") })
	cmd.PersistentFlags().String("config", "", "节点连接配置，仅serve读取")
	cmd.Args = func(_ *cobra.Command, args []string) error {
		if len(args) > 0 {
			return errors.New("命令无效")
		}
		return nil
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error { return cmd.Help() }
	cmd.AddCommand(version.NewCommand(), newDoctorCommand(), newServeCommand())
	return cmd
}
func newDoctorCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "doctor", Short: "只读检查本机工具与节点记录", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		dataDir, _ := cmd.Flags().GetString("data-dir")
		if strings.TrimSpace(dataDir) == "" {
			return errors.New("节点数据目录不合法")
		}
		if dataDir == "~" || strings.HasPrefix(dataDir, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return errors.New("节点数据目录不合法")
			}
			if dataDir == "~" {
				dataDir = home
			} else {
				dataDir = filepath.Join(home, strings.TrimPrefix(dataDir, "~/"))
			}
		}
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		report, doctorErr := node.Doctor(ctx, dataDir)
		jsonOutput, _ := cmd.Flags().GetBool("json")
		if jsonOutput {
			if err := json.NewEncoder(cmd.OutOrStdout()).Encode(report); err != nil {
				return errors.New("写入节点检查失败")
			}
		} else {
			for _, tool := range report.Tools {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: %s %s %s\n", tool.Name, tool.Status, tool.Version, tool.Reason); err != nil {
					return errors.New("写入节点检查失败")
				}
			}
		}
		if doctorErr != nil {
			return doctorErr
		}
		for _, tool := range report.Tools {
			if tool.Status == "failed" {
				return errors.New("节点本机检查未通过")
			}
		}
		return nil
	}}
	cmd.Flags().String("data-dir", "~/.mybuilds/agent", "本地节点数据目录")
	cmd.Flags().Bool("json", false, "输出JSON检查记录")
	return cmd
}
