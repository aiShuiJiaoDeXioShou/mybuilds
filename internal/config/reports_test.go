package config

import (
	"fmt"
	"strings"
	"testing"
)

func TestReportsPatternAndPathLimits(t *testing.T) {
	longPath := strings.Repeat("a/", 385) + strings.Repeat("b", 250) + ".xml"
	for _, tc := range []struct {
		name  string
		paths []string
		valid bool
	}{
		{"32模式", repeatedReportPaths(32), true},
		{"33模式", repeatedReportPaths(33), false},
		{"1024字节路径", []string{longPath}, true},
		{"1025字节路径", []string{longPath + "x"}, false},
		{"255字节叶名称", []string{strings.Repeat("a", 251) + ".xml"}, true},
		{"256字节叶名称", []string{strings.Repeat("a", 252) + ".xml"}, false},
		{"多字节叶名称", []string{strings.Repeat("报告", 42) + ".xml"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var source strings.Builder
			source.WriteString(minimal + "reports:\n  junit:\n    paths:\n")
			for _, value := range tc.paths {
				fmt.Fprintf(&source, "      - %q\n", value)
			}
			_, err := Parse([]byte(source.String()))
			if (err == nil) != tc.valid {
				t.Fatalf("有效性=%v，错误=%v", tc.valid, err)
			}
		})
	}
}

func repeatedReportPaths(count int) []string {
	paths := make([]string, count)
	for i := range paths {
		paths[i] = fmt.Sprintf("results/%d/*.xml", i)
	}
	return paths
}

func TestReportsStrictSchemaAndRequired(t *testing.T) {
	for _, value := range []string{"", ", required: true", ", required: false"} {
		d, err := Parse([]byte(minimal + "reports: {junit: {paths: ['results/*.xml']" + value + "}}\n"))
		if err != nil {
			t.Fatal(err)
		}
		required := d.Builds["default"].Reports.JUnit.Required
		if value == "" {
			// nil按冻结契约由执行消费者视为true，加载和预览保持原始定义。
			if required != nil {
				t.Fatal("省略的required被改写")
			}
		} else if required == nil || *required != (value == ", required: true") {
			t.Fatal("显式required丢失")
		}
	}
	for _, report := range []string{
		"{junit: {paths: ['results/*.xml'], required: 'true'}}",
		"{junit: {paths: ['results/*.xml'], required: null}}",
		"{junit: {paths: ['results/*.xml'], required: true, required: false}}",
		"{junit: {paths: ['results/*.xml'], unknown: SECRET_REPORT_INPUT}}",
		"{junit: {paths: ['results/*.xml'], Required: true}}",
		"{junit: {paths: null}}", "{junit: {paths: [7]}}", "{junit: null}",
	} {
		_, err := Parse([]byte(minimal + "reports: " + report + "\n"))
		if err == nil || strings.Contains(err.Error(), "SECRET_REPORT_INPUT") {
			t.Fatalf("未安全拒绝非法报告配置：%v", err)
		}
	}
}
