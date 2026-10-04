package config

import (
	"slices"
	"strings"
)

type BuildSettings struct {
	Params map[string]string `yaml:"params,omitempty" json:"params,omitempty"`
}
type PipelineSettings struct {
	Source string                   `yaml:"source,omitempty" json:"source,omitempty"`
	File   string                   `yaml:"file,omitempty" json:"file,omitempty"`
	Builds map[string]BuildSettings `yaml:"builds,omitempty" json:"builds,omitempty"`
	Params map[string]string        `yaml:"params,omitempty" json:"params,omitempty"`
}
type ProjectSettings struct {
	Pipeline *PipelineSettings `yaml:"pipeline,omitempty" json:"pipeline,omitempty"`
}

func ParseProjectSettings(data []byte) (ProjectSettings, error) {
	var settings ProjectSettings
	if err := decodeConfiguration(data, &settings); err != nil {
		return settings, err
	}
	if settings.Pipeline != nil {
		if settings.Pipeline.Source == "" {
			settings.Pipeline.Source = "auto"
		}
		if settings.Pipeline.File == "" {
			settings.Pipeline.File = "mybuilds.yml"
		}
	}
	return settings, ValidateProjectSettings(settings)
}

// ValidateProjectSettings不读取Git或节点秘密，默认值由解析与实际业务入口补全。
func ValidateProjectSettings(settings ProjectSettings) error {
	p := settings.Pipeline
	if p == nil {
		return nil
	}
	if p.Source != "" && p.Source != "auto" && p.Source != "repo" {
		return invalid("pipeline.source", "尚未支持指定来源")
	}
	if p.File != "" {
		if err := validatePath(p.File, false, "pipeline.file"); err != nil {
			return err
		}
		if strings.Contains(p.File, "{{") {
			return invalid("pipeline.file", "不允许模板")
		}
	}
	if p.Params != nil && p.Builds != nil {
		return invalid("pipeline", "不能混写旧参数与命名builds")
	}
	if len(p.Builds) > 64 {
		return invalid("pipeline.builds", "超过数量上限")
	}
	for _, name := range sortedKeys(p.Builds) {
		if !safeName(name) || len(name) > 64 {
			return invalid("pipeline.builds", "名称不合法")
		}
		if err := validateSettingsParams(p.Builds[name].Params); err != nil {
			return err
		}
	}
	return validateSettingsParams(p.Params)
}
func validateSettingsParams(params map[string]string) error {
	if len(params) > 128 {
		return invalid("pipeline.params", "超过数量上限")
	}
	for _, name := range sortedKeys(params) {
		if !identifier.MatchString(name) || slices.Contains([]string{"project", "workspace"}, name) {
			return invalid("pipeline.params", "参数名称不合法")
		}
		if len(params[name]) > 4096 {
			return invalid("pipeline.params", "参数值超过上限")
		}
	}
	return nil
}

// LoadProjectSettings供本机与远程CLI读取同一严格settings输入。
func LoadProjectSettings(filename string) (ProjectSettings, error) {
	if filename == "" {
		return ProjectSettings{}, invalid("项目settings", "需要明确文件名")
	}
	path, err := configurationPath(filename, "settings.yml")
	if err != nil {
		return ProjectSettings{}, err
	}
	data, _, err := readConfiguration(path)
	if err != nil {
		return ProjectSettings{}, invalid("项目settings", "无法读取普通文件")
	}
	return ParseProjectSettings(data)
}
