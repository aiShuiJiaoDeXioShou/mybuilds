package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRetentionServerDefaultsAndOverrides(t *testing.T) {
	clearServerEnvironment(t)
	for _, item := range []struct {
		name, body   string
		builds, days int64
	}{
		{"omitted", "concurrency: 1\n", 100, 30},
		{"empty", "retention: {}\n", 100, 30},
		{"builds", "retention: {builds: 200}\n", 200, 30},
		{"days", "retention: {days: 45}\n", 100, 45},
		{"both", "retention: {builds: 200, days: 45}\n", 200, 45},
		{"bounds", "retention: {builds: 9223372036854775807, days: 106751}\n", 9223372036854775807, 106751},
	} {
		t.Run(item.name, func(t *testing.T) {
			cfg, err := LoadServer(ServerLoadOptions{Filename: writeManagementConfig(t, item.body), Explicit: true})
			if err != nil || cfg.Retention.Builds != item.builds || cfg.Retention.Days != item.days {
				t.Fatalf("有效全局值错误: %+v %v", cfg.Retention, err)
			}
		})
	}
	cfg, err := LoadServer(ServerLoadOptions{Filename: filepath.Join(t.TempDir(), "missing.yml")})
	if err != nil || cfg.Retention != (Retention{Builds: 100, Days: 30}) {
		t.Fatal("缺省文件策略错误", err)
	}
}

func TestRetentionProjectFieldInheritance(t *testing.T) {
	for _, item := range []struct {
		name, body   string
		builds, days *int64
		present      bool
	}{
		{"omitted", "{}\n", nil, nil, false},
		{"reset", "retention: {}\n", nil, nil, true},
		{"builds", "retention: {builds: 200}\n", retentionTestInt(200), nil, true},
		{"days", "retention: {days: 45}\n", nil, retentionTestInt(45), true},
		{"both", "retention: {builds: 200, days: 45}\n", retentionTestInt(200), retentionTestInt(45), true},
	} {
		t.Run(item.name, func(t *testing.T) {
			settings, err := ParseProjectSettings([]byte(item.body))
			if err != nil {
				t.Fatal(err)
			}
			if (settings.Retention != nil) != item.present {
				t.Fatal("空块与省略语义错误")
			}
			if !item.present {
				return
			}
			if !retentionTestSame(settings.Retention.Builds, item.builds) || !retentionTestSame(settings.Retention.Days, item.days) {
				t.Fatal("字段未按省略保留继承")
			}
			file := writeManagementConfig(t, item.body)
			loaded, err := LoadProjectSettings(file)
			if err != nil || loaded.Retention == nil || !retentionTestSame(loaded.Retention.Builds, item.builds) || !retentionTestSame(loaded.Retention.Days, item.days) {
				t.Fatal("实际文件导入改变覆盖语义", err)
			}
		})
	}
}

func TestRetentionStrictWholeConfiguration(t *testing.T) {
	clearServerEnvironment(t)
	for _, body := range []string{
		"retention: {builds: 0}\n", "retention: {builds: -1}\n", "retention: {builds: 1.5}\n", "retention: {builds: 1.0}\n", "retention: {builds: '1'}\n", "retention: {builds: true}\n", "retention: {builds: null}\n", "retention: {builds: 9223372036854775808}\n",
		"retention: {days: 0}\n", "retention: {days: -1}\n", "retention: {days: 106752}\n", "retention: {days: 1.5}\n", "retention: {days: '1'}\n", "retention: {days: null}\n", "retention: {days: 9223372036854775808}\n",
		"retention: null\n", "retention: []\n", "retention: {unknown: RETENTION_SECRET_INPUT}\n", "retention: {builds: 200, builds: 300}\n", "retention: {}\nretention: {}\n", "retention: {builds: 200, days: 0}\n",
	} {
		if _, err := LoadServer(ServerLoadOptions{Filename: writeManagementConfig(t, body), Explicit: true}); err == nil || strings.Contains(err.Error(), "RETENTION_SECRET_INPUT") {
			t.Fatalf("全局非法配置被接受/泄露: %q %v", body, err)
		}
		if _, err := ParseProjectSettings([]byte(body)); err == nil || strings.Contains(err.Error(), "RETENTION_SECRET_INPUT") {
			t.Fatalf("项目非法配置被接受/泄露: %q %v", body, err)
		}
	}
}

func TestRetentionRepositoryDocumentRejectsManagementPolicy(t *testing.T) {
	_, err := Parse([]byte("version: 1\nretention: {builds: 100, days: 30}\nbuilds:\n  default:\n    steps:\n      - kind: run\n        run: 'true'\n"))
	if err == nil {
		t.Fatal("仓库流水线接受管理保留策略")
	}
}

func retentionTestInt(v int64) *int64 { return &v }
func retentionTestSame(a, b *int64) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}
