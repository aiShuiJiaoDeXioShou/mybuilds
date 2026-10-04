package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"mybuilds/internal/protocol"
	"mybuilds/internal/server"
)

func newLogsCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "logs <id>", Short: "读取中央确认日志", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		id, e := uuid.Parse(args[0])
		if e != nil || id.String() != args[0] {
			return errors.New("构建标识无效")
		}
		after, _ := cmd.Flags().GetInt64("after-seq")
		limit, _ := cmd.Flags().GetInt("limit")
		step, _ := cmd.Flags().GetString("step")
		follow, _ := cmd.Flags().GetBool("follow")
		jsonOutput, _ := cmd.Flags().GetBool("json")
		timeout, _ := cmd.Flags().GetDuration("stream-timeout")
		if after < 0 || limit < 1 || limit > 200 || timeout < time.Second || timeout > time.Hour || follow && jsonOutput {
			return errors.New("日志选项无效")
		}
		query := url.Values{"after_seq": {strconv.FormatInt(after, 10)}, "limit": {strconv.Itoa(limit)}}
		if step != "" {
			query.Set("step", step)
		}
		if follow {
			live, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			ctx, cancel := context.WithTimeout(live, timeout)
			defer cancel()
			return followRemoteLogs(ctx, cmd, args[0], query, after)
		}
		var result server.LogPage
		if e := remoteRequest(cmd, http.MethodGet, "/api/builds/"+args[0]+"/log?"+query.Encode(), nil, &result, ""); e != nil {
			return e
		}
		if jsonOutput {
			return remoteOutput(cmd, result, nil, nil)
		}
		for _, record := range result.Records {
			if e := printLogRecord(cmd, record); e != nil {
				return e
			}
		}
		return nil
	}}
	cmd.Flags().String("step", "", "按冻结步骤名称筛选")
	cmd.Flags().Int64("after-seq", 0, "从已确认chunk序号续读")
	cmd.Flags().Int("limit", 200, "每页chunk数，最大200")
	cmd.Flags().BoolP("follow", "f", false, "持续读取中央确认日志")
	cmd.Flags().Duration("stream-timeout", 15*time.Minute, "日志流总时长，1s至1h")
	cmd.Flags().Bool("json", false, "历史日志输出JSON")
	return cmd
}
func safeLogText(text string) string {
	var out strings.Builder
	for _, r := range text {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			fmt.Fprintf(&out, "\\u%04x", r)
		} else {
			out.WriteRune(r)
		}
	}
	return out.String()
}
func printLogRecord(cmd *cobra.Command, r protocol.LogRecord) error {
	_, e := fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s[%d] %s %s: %s\n", r.UTC.UTC().Format(time.RFC3339Nano), safeLogText(r.Build), safeLogText(r.Phase), r.Index, safeLogText(r.Step), safeLogText(r.Stream), safeLogText(r.Text))
	if e != nil {
		return errors.New("日志输出失败")
	}
	return nil
}
