package client

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/spf13/cobra"
	"mybuilds/internal/server"
	"net/http"
	"net/url"
	"strconv"
)

func newTriggerCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "trigger <project>", Short: "固定提交并原子排队，不执行构建", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		builds, _ := cmd.Flags().GetStringSlice("build")
		all, _ := cmd.Flags().GetBool("all")
		if all && cmd.Flags().Changed("build") {
			return errors.New("build与all不能同时指定")
		}
		parameters, _ := cmd.Flags().GetStringArray("param")
		for _, flag := range []string{"version", "channel"} {
			if cmd.Flags().Changed(flag) {
				value, _ := cmd.Flags().GetString(flag)
				parameters = append(parameters, flag+"="+value)
			}
		}
		shared, scoped, err := triggerParameters(parameters, "", "")
		if err != nil {
			return err
		}
		var request server.TriggerRequest
		request.BuildNames = builds
		request.All = all
		request.Params = shared
		request.BuildParams = scoped
		request.Branch, _ = cmd.Flags().GetString("branch")
		request.Ref, _ = cmd.Flags().GetString("ref")
		request.AllowUpload, _ = cmd.Flags().GetBool("allow-upload")
		key, _ := cmd.Flags().GetString("idempotency-key")
		if cmd.Flags().Changed("idempotency-key") {
			if key == "" || len(key) > 128 {
				return errors.New("幂等key不合法")
			}
			for _, r := range key {
				if r < 33 || r > 126 {
					return errors.New("幂等key不合法")
				}
			}
		} else {
			bytes := make([]byte, 32)
			if _, err := rand.Read(bytes); err != nil {
				return errors.New("生成幂等key失败")
			}
			key = hex.EncodeToString(bytes)
		}
		// 在网络开始前交付非机密key；超时后用户可显式重用，客户端不换key重试。
		if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "request_key: %s\n", key); err != nil {
			return errors.New("写入幂等key失败")
		}
		var result server.BatchView
		if err := remoteRequest(cmd, http.MethodPost, "/api/projects/"+url.PathEscape(args[0])+"/builds", request, &result, key); err != nil {
			return err
		}

		output := struct {
			RequestKey string `json:"request_key"`
			server.BatchView
		}{key, result}
		rows := make([][]string, 0, len(result.Builds))
		for _, v := range result.Builds {
			number := "-"
			if v.Number != nil {
				number = strconv.FormatInt(*v.Number, 10)
			}
			rows = append(rows, []string{result.ID, v.ID, v.Name, number, v.Status, result.SHA})
		}
		return remoteOutput(cmd, output, []string{"BATCH", "ID", "BUILD", "NUMBER", "STATUS", "SHA"}, rows)

	}}
	cmd.Flags().String("branch", "main", "允许分支")
	cmd.Flags().String("ref", "", "完整commit SHA")
	cmd.Flags().StringSlice("build", nil, "选择命名build")
	cmd.Flags().Bool("all", false, "选择全部build")
	cmd.Flags().StringArray("param", nil, "共享key=value或build:key=value")
	cmd.Flags().String("version", "", "共享版本参数")
	cmd.Flags().String("channel", "", "共享渠道参数")
	cmd.Flags().Bool("allow-upload", false, "显式允许发布排队，仍须admin")
	cmd.Flags().String("idempotency-key", "", "重用一次请求的非机密key")
	cmd.Flags().Bool("json", false, "输出JSON")
	return cmd
}
