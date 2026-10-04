package config

import (
	"fmt"
	"path"
	"strings"
	"unicode"
)

// RenderField 只解释原字段，供预览、执行与Store同一次模板规则使用。
func RenderField(value, field string, params, context map[string]string, secrets, notification bool) (string, bool, error) {
	if secrets {
		for rest := value; ; {
			start := strings.Index(rest, "${")
			if start < 0 {
				break
			}
			rest = rest[start+2:]
			end := strings.IndexByte(rest, '}')
			if end < 0 || !identifier.MatchString(rest[:end]) {
				return "", false, fmt.Errorf("%s: 环境引用格式错误", field)
			}
			rest = rest[end+1:]
		}
	}
	missing := false
	var rendered strings.Builder
	for rest := value; ; {
		start := strings.Index(rest, "{{")
		close := strings.Index(rest, "}}")
		if start < 0 {
			if close >= 0 {
				return "", false, fmt.Errorf("%s: 模板格式错误", field)
			}
			rendered.WriteString(rest)
			break
		}
		if close >= 0 && close < start {
			return "", false, fmt.Errorf("%s: 模板格式错误", field)
		}
		rendered.WriteString(rest[:start])
		rest = rest[start+2:]
		end := strings.Index(rest, "}}")
		if end < 0 {
			return "", false, fmt.Errorf("%s: 模板格式错误", field)
		}
		key := strings.TrimSpace(rest[:end])
		if value, exists := params[key]; exists {
			rendered.WriteString(value)
		} else {
			switch key {
			case "project", "build.name", "build.number", "build.id", "git.sha", "git.branch", "node.name", "workspace", "step.name":
			case "build.status", "build.url":
				if !notification {
					return "", false, fmt.Errorf("%s: 未知模板变量", field)
				}
			default:
				return "", false, fmt.Errorf("%s: 未知模板变量", field)
			}
			if value, known := context[key]; known {
				rendered.WriteString(value)
			} else {
				missing = true
			}
		}
		rest = rest[end+2:]
	}
	if !missing && isPathField(field) {
		renderedValue := rendered.String()
		if value != "" && strings.TrimSpace(renderedValue) == "" {
			return "", false, fmt.Errorf("%s: 模板渲染后路径不能为空", field)
		}
		value := renderedValue
		if strings.HasSuffix(field, ".reports.junit.paths") && (len(value) > 1024 || len(path.Base(value)) > 255) {
			return "", false, fmt.Errorf("%s: 模板渲染后报告路径或叶名称超过大小上限", field)
		}
		if strings.Contains(value, "\\") {
			return "", false, fmt.Errorf("%s: 模板渲染后路径不允许反斜杠", field)
		}
		for _, r := range value {
			if unicode.IsControl(r) {
				return "", false, fmt.Errorf("%s: 模板渲染后路径不能含控制字符", field)
			}
		}
		for _, part := range strings.Split(value, "/") {
			if part == ".." {
				return "", false, fmt.Errorf("%s: 模板渲染后路径不得包含向上段", field)
			}
		}
		if strings.HasSuffix(field, ".paths") || strings.HasSuffix(field, ".file") {
			if _, err := path.Match(value, ""); err != nil {
				return "", false, fmt.Errorf("%s: 模板渲染后路径模式无效", field)
			}
		} else if strings.ContainsAny(value, "*?[") {
			return "", false, fmt.Errorf("%s: 模板渲染后路径不允许 glob", field)
		}
		if path.IsAbs(value) || (len(value) > 1 && value[1] == ':') {
			return "", false, fmt.Errorf("%s: 模板渲染后路径必须相对且不得越界", field)
		}
	}
	return rendered.String(), missing, nil
}

func isPathField(field string) bool {
	for _, suffix := range []string{".working_dir", ".file", ".result_file", ".paths"} {
		if strings.HasSuffix(field, suffix) {
			return true
		}
	}
	return false
}
