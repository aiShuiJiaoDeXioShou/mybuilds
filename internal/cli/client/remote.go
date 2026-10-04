package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/spf13/cobra"
	"io"
	"mybuilds/internal/config"
	"net/http"
	"text/tabwriter"
	"time"
)

// remoteRequest仅做当前API传输，不自动重试或执行仓库内容。
func remoteRequest(cmd *cobra.Command, method, path string, body, response any, key string) error {
	filename, _ := cmd.Flags().GetString("config")
	options := config.ClientLoadOptions{Filename: filename, Explicit: cmd.Flags().Changed("config")}
	if cmd.Flags().Changed("server-url") {
		server, _ := cmd.Flags().GetString("server-url")
		options.ServerURL = &server
	}
	if cmd.Flags().Changed("timeout") {
		timeout, _ := cmd.Flags().GetDuration("timeout")
		options.Timeout = &timeout
	}
	cfg, err := config.LoadClient(options)
	if err != nil {
		return err
	}
	var data []byte
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil || len(data) > config.MaxConfigBytes {
			return errors.New("请求内容不合法")
		}
	}
	req, err := http.NewRequestWithContext(cmd.Context(), method, cfg.Server+path, bytes.NewReader(data))
	if err != nil {
		return errors.New("无法建立API请求")
	}
	req.Header.Set("Authorization", "Bearer "+cfg.RuntimeToken)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: cfg.Timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	result, err := client.Do(req)
	if err != nil {
		return errors.New("控制端API请求失败")
	}
	defer result.Body.Close()
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		return errors.New("控制端拒绝API请求")
	}
	if result.StatusCode == http.StatusNoContent {
		return nil
	}
	data, err = io.ReadAll(io.LimitReader(result.Body, config.MaxConfigBytes+1))
	if err != nil || len(data) > config.MaxConfigBytes {
		return errors.New("控制端响应无效")
	}
	if response == nil {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(response) != nil {
		return errors.New("控制端响应无效")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("控制端响应无效")
	}
	return nil
}
func remoteOutput(cmd *cobra.Command, value any, columns []string, rows [][]string) error {
	asJSON, _ := cmd.Flags().GetBool("json")
	if asJSON {
		if err := json.NewEncoder(cmd.OutOrStdout()).Encode(value); err != nil {
			return errors.New("写入API结果失败")
		}
		return nil
	}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	for _, row := range append([][]string{columns}, rows...) {
		for i, item := range row {
			if i > 0 {
				fmt.Fprint(w, "\t")
			}
			fmt.Fprint(w, item)
		}
		fmt.Fprintln(w)
	}
	if err := w.Flush(); err != nil {
		return errors.New("写入API结果失败")
	}
	return nil
}
func remotePageFlags(cmd *cobra.Command) {
	cmd.Flags().Int("limit", 20, "每页数量，最大200")
	cmd.Flags().Int("offset", 0, "分页起点")
	cmd.Flags().Bool("json", false, "输出JSON")
}
func remotePage(cmd *cobra.Command) (int, int, error) {
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")
	if limit < 1 || limit > 200 || offset < 0 || offset > 1000000 {
		return 0, 0, errors.New("分页参数不合法")
	}
	return limit, offset, nil
}
func remoteRootFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String("config", "", "客户端连接配置")
	cmd.PersistentFlags().String("server-url", "", "控制端URL")
	cmd.PersistentFlags().Duration("timeout", 30*time.Second, "远程API超时")
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, _ error) error { return errors.New("命令选项不合法") })
}
