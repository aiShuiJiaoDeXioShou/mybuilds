package config

import (
	"slices"
	"strings"
)

// BindProfiles只生成typed项目绑定，不读取配置、工程或工具。
func BindProfiles(framework, platforms string) (*PipelineSettings, error) {
	if framework == "" {
		framework = "native"
	}
	if framework != "native" && framework != "flutter" {
		return nil, invalid("framework", "只允许native/flutter")
	}
	names := strings.Split(platforms, ",")
	seen := map[string]bool{}
	for _, platform := range names {
		if (platform != "android" && platform != "ios") || seen[platform] {
			return nil, invalid("platform", "只允许不重复android/ios")
		}
		seen[platform] = true
	}
	builds := map[string]BuildSettings{}
	for _, platform := range []string{"android", "ios"} {
		if seen[platform] {
			builds[platform] = BuildSettings{Profile: framework + "-" + platform}
		}
	}
	return &PipelineSettings{Source: "auto", File: "mybuilds.yml", Builds: builds}, nil
}

type ServerDefaults struct {
	Notifications *Notifications `yaml:"notifications,omitempty" mapstructure:"notifications"`
}

// ResolveNotifications按字段覆盖；列表整体替换，返回独立值供原consumer使用。
func ResolveNotifications(project, pipeline, defaults *Notifications) *Notifications {
	if project == nil && pipeline == nil && defaults == nil {
		return nil
	}
	out := &Notifications{}
	for _, n := range []*Notifications{defaults, pipeline, project} {
		if n == nil {
			continue
		}
		if n.Enabled != nil {
			value := *n.Enabled
			out.Enabled = &value
		}
		if n.On != nil {
			out.On = slices.Clone(n.On)
		}
		if n.Webhooks != nil {
			out.Webhooks = slices.Clone(n.Webhooks)
		}
		if n.Template != "" {
			out.Template = n.Template
		}
	}
	return out
}

// LoadTemplate与本地init共用真实受限读；完整named文档仍合法。
func LoadTemplate(filename string) ([]byte, error) {
	data, err := readProfileFile(filename)
	if err != nil {
		return nil, invalid("模板", "无法读取普通文件")
	}
	if _, err := Parse(data); err != nil {
		return nil, err
	}
	return data, nil
}
