package client

import (
	"errors"
	"regexp"
	"strings"
)

var parameterName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func triggerParameters(values []string, version, channel string) (map[string]string, map[string]map[string]string, error) {
	shared := map[string]string{}
	scoped := map[string]map[string]string{}
	if version != "" {
		values = append(append([]string{}, values...), "version="+version)
	}
	if channel != "" {
		values = append(append([]string{}, values...), "channel="+channel)
	}
	if len(values) > 128 {
		return nil, nil, errors.New("参数数量超过上限")
	}
	for _, value := range values {
		key, content, ok := strings.Cut(value, "=")
		if !ok || len(content) > 4096 {
			return nil, nil, errors.New("参数格式不合法")
		}
		target := shared
		if build, name, scopedKey := strings.Cut(key, ":"); scopedKey {
			if build == "" || len(build) > 64 || strings.ContainsAny(build, "/:\\\x00") {
				return nil, nil, errors.New("命名参数scope不合法")
			}
			key = name
			if scoped[build] == nil {
				scoped[build] = map[string]string{}
			}
			target = scoped[build]
		}
		if !parameterName.MatchString(key) {
			return nil, nil, errors.New("参数名称不合法")
		}
		if _, exists := target[key]; exists {
			return nil, nil, errors.New("同scope参数不能重复")
		}
		target[key] = content
	}
	return shared, scoped, nil
}
