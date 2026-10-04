package client

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"mybuilds/internal/server"
)

// 重试必须显式保留请求key，不覆盖原提交、参数或条件，不自动重发。
func newRetryCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "retry <id>", Short: "按原快照创建新的构建", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		id, err := uuid.Parse(args[0])
		if err != nil || id == uuid.Nil || id.String() != args[0] {
			return errors.New("构建标识无效")
		}
		key, _ := cmd.Flags().GetString("idempotency-key")
		if !cmd.Flags().Changed("idempotency-key") || key == "" || len(key) > 128 {
			return errors.New("重试必须指定有效的幂等key")
		}
		for _, r := range key {
			if r < 33 || r > 126 || r == '/' || r == '\\' {
				return errors.New("幂等key不合法")
			}
		}
		input := struct {
			AllowUpload bool `json:"allow_upload"`
		}{}
		input.AllowUpload, _ = cmd.Flags().GetBool("allow-upload")
		if _, err = fmt.Fprintf(cmd.ErrOrStderr(), "request_key: %s\n", key); err != nil {
			return errors.New("写入幂等key失败")
		}
		var result server.BatchView
		if err = remoteRequest(cmd, http.MethodPost, "/api/builds/"+url.PathEscape(args[0])+"/retry", input, &result, key); err != nil {
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
			rows = append(rows, []string{result.ID, v.ID, v.Name, number, v.Status, v.RetryOf, result.SHA})
		}
		return remoteOutput(cmd, output, []string{"BATCH", "ID", "BUILD", "NUMBER", "STATUS", "RETRY_OF", "SHA"}, rows)
	}}
	cmd.Flags().String("idempotency-key", "", "明确重用本次请求的非机密key")
	cmd.Flags().Bool("allow-upload", false, "明确允许原定义中的发布，仍须admin")
	cmd.Flags().Bool("json", false, "输出JSON")
	return cmd
}
