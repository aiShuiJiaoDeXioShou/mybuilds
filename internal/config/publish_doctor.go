package config

import (
	"path/filepath"
)

// 诊断只读取明确的工具配置；不解析、加载或覆盖控制端身份token。
func LoadPublishDoctor(filename string) (PublishTools, string, error) {
	if filename == "" {
		return PublishTools{}, "", invalid("发布诊断", "需要显式Agent配置")
	}
	path, err := configurationPath(filename, "agent.yml")
	if err != nil {
		return PublishTools{}, "", err
	}
	data, info, err := readConfiguration(path)
	if err != nil {
		return PublishTools{}, "", invalid("发布诊断", "无法读取普通配置文件")
	}
	var file agentFile
	if err = decodeConfiguration(data, &file); err != nil {
		return PublishTools{}, "", err
	}
	if file.Token != "" && (info.Mode().Perm() != 0600 || !configurationOwned(info)) {
		return PublishTools{}, "", invalid("发布诊断", "身份配置需要自有0600文件")
	}
	if file.PublishTools == nil || file.PublishTools.BundleDir == "" {
		return PublishTools{}, "", invalid("发布诊断", "缺少发布工具配置")
	}
	base := filepath.Dir(path)
	bundle, err := expandConfigurationPath(file.PublishTools.BundleDir, base)
	if err != nil {
		return PublishTools{}, "", err
	}
	jar := ""
	if file.PublishTools.Bundletool != "" {
		jar, err = expandConfigurationPath(file.PublishTools.Bundletool, base)
		if err != nil {
			return PublishTools{}, "", err
		}
	}
	dir := file.DataDir
	if dir == "" {
		dir = "~/.mybuilds/agent"
	}
	dir, err = expandConfigurationPath(dir, base)
	return PublishTools{BundleDir: bundle, Bundletool: jar}, dir, err
}
