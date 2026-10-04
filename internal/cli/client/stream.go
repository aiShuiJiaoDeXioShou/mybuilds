package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"mybuilds/internal/server"
)

func followRemoteLogs(ctx context.Context, cmd *cobra.Command, build string, query url.Values, last int64) error {
	cfg, e := remoteSettings(cmd)
	if e != nil {
		return e
	}
	transport, e := remoteTransport(cfg)
	if e != nil {
		return e
	}
	defer transport.CloseIdleConnections()
	transport.ResponseHeaderTimeout = cfg.Timeout
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for {
		if ctx.Err() != nil {
			return nil
		}
		query.Set("follow", "1")
		query.Set("after_seq", strconv.FormatInt(last, 10))
		request, e := http.NewRequestWithContext(ctx, "GET", cfg.Server+"/api/builds/"+build+"/log?"+query.Encode(), nil)
		if e != nil {
			return errors.New("日志流请求无效")
		}
		request.Header.Set("Authorization", "Bearer "+cfg.RuntimeToken)
		request.Header.Set("Accept", "text/event-stream")
		request.Header.Set("Last-Event-ID", strconv.FormatInt(last, 10))
		response, e := client.Do(request)
		if e != nil {
			if ctx.Err() != nil {
				return nil
			}
			return errors.New("日志流请求失败")
		}
		if response.StatusCode != 200 || strings.Split(response.Header.Get("Content-Type"), ";")[0] != "text/event-stream" {
			response.Body.Close()
			return errors.New("控制端拒绝日志流")
		}
		ended, e := readLogEvents(ctx, cmd, response.Body, &last)
		response.Body.Close()
		if ctx.Err() != nil {
			return nil
		}
		if e != nil {
			return e
		}
		if ended {
			return nil
		}
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}
func readLogEvents(ctx context.Context, cmd *cobra.Command, body io.Reader, last *int64) (bool, error) {
	reader := bufio.NewReaderSize(body, 64*1024+1)
	fields := map[string]string{}
	size := 0
	lastOffset := int64(-1)
	for {
		line, e := reader.ReadSlice('\n')
		if e == io.EOF && len(line) == 0 && size == 0 {
			return false, nil
		}
		if e != nil {
			return false, errors.New("日志流响应不完整")
		}
		size += len(line)
		if size > 64*1024 {
			return false, errors.New("日志流事件超过上限")
		}
		if ctx.Err() != nil {
			return false, nil
		}
		text := strings.TrimSuffix(strings.TrimSuffix(string(line), "\n"), "\r")
		if text != "" {
			if strings.HasPrefix(text, ":") {
				continue
			}
			key, value, ok := strings.Cut(text, ":")
			if !ok || (key != "event" && key != "id" && key != "data") {
				return false, errors.New("日志流事件无效")
			}
			if _, exists := fields[key]; exists {
				return false, errors.New("日志流事件无效")
			}
			fields[key] = strings.TrimPrefix(value, " ")
			continue
		}
		if len(fields) == 0 {
			size = 0
			continue
		}
		if fields["event"] == "end" {
			if len(fields) != 2 || fields["data"] != "{}" {
				return false, errors.New("日志流终态无效")
			}
			return true, nil
		}
		if fields["event"] != "log" || len(fields) != 3 {
			return false, errors.New("日志流事件无效")
		}
		seq, e := strconv.ParseInt(fields["id"], 10, 64)
		if e != nil || seq < 1 || fields["id"] != strconv.FormatInt(seq, 10) {
			return false, errors.New("日志流序号无效")
		}
		var page server.LogPage
		decoder := json.NewDecoder(bytes.NewReader([]byte(fields["data"])))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&page) != nil || decoder.Decode(new(any)) != io.EOF || page.Records == nil || len(page.Records) > 16 || page.NextSeq != seq || page.NextOffset < 0 {
			return false, errors.New("日志流数据无效")
		}
		if seq <= *last {
			fields = map[string]string{}
			size = 0
			continue
		}
		if *last == math.MaxInt64 || seq != *last+1 || page.NextOffset <= lastOffset {
			return false, errors.New("日志流序号或偏移无效")
		}
		for _, r := range page.Records {
			_, offset := r.UTC.Zone()
			if r.UTC.IsZero() || offset != 0 || r.Index < 1 || len(r.Text) > 8192 || (r.Stream != "stdout" && r.Stream != "stderr" && r.Stream != "system") {
				return false, errors.New("日志流记录无效")
			}
			if e = printLogRecord(cmd, r); e != nil {
				return false, e
			}
		}
		*last = seq
		lastOffset = page.NextOffset
		fields = map[string]string{}
		size = 0
	}
}
