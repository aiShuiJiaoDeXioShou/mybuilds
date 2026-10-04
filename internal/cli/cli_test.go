package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"mybuilds/internal/cli/client"
	"mybuilds/internal/cli/server"
)

func TestCLIContract(t *testing.T) {
	for name, newCommand := range map[string]func() *cobra.Command{
		"mybuilds":        client.NewCommand,
		"mybuilds-server": server.NewCommand,
	} {
		for _, tc := range []struct {
			name    string
			args    []string
			want    string
			wantErr bool
		}{
			{"帮助", []string{"--help"}, name, false},
			{"默认帮助", []string{}, "version", false},
			{"版本", []string{"version"}, "dev (commit: unknown, built: unknown)\n", false},
			{"未知命令", []string{"does-not-exist"}, "", true},
			{"多余参数", []string{"version", "extra"}, "", true},
		} {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				cmd := newCommand()
				var out bytes.Buffer
				cmd.SetOut(&out)
				cmd.SetErr(&out)
				cmd.SetArgs(tc.args)
				err := cmd.Execute()
				if (err != nil) != tc.wantErr {
					t.Fatalf("错误 = %v，需要错误 = %v", err, tc.wantErr)
				}
				if !strings.Contains(out.String(), tc.want) {
					t.Fatalf("输出 %q 未包含 %q", out.String(), tc.want)
				}
				if tc.wantErr && strings.Contains(out.String(), "commit:") {
					t.Fatalf("错误参数却输出版本：%q", out.String())
				}
			})
		}
	}
}
