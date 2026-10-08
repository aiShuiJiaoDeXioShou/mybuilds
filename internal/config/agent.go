package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// AgentConfig仅由serve消费；运行凭据及其环境来源不进入公开JSON。
type PublishTools struct {
	BundleDir  string `yaml:"bundle_dir" mapstructure:"bundle_dir" json:"-"`
	Bundletool string `yaml:"bundletool" mapstructure:"bundletool" json:"-"`
}

type AgentConfig struct {
	PublishTools                               *PublishTools `json:"-"`
	Server, Node, DataDir, SecretsFile, CAFile string
	Capacity                                   int
	HeartbeatInterval, LeaseDuration           time.Duration
	RuntimeToken, TokenEnv                     string `json:"-"`
}
type AgentLoadOptions struct {
	Filename string
	Explicit bool
}
type agentFile struct {
	PublishTools      *PublishTools `yaml:"publish_tools" mapstructure:"publish_tools"`
	Server            string        `yaml:"server" mapstructure:"server"`
	Node              string        `yaml:"node" mapstructure:"node"`
	Token             string        `yaml:"token" mapstructure:"token"`
	Capacity          int           `yaml:"capacity" mapstructure:"capacity"`
	DataDir           string        `yaml:"data_dir" mapstructure:"data_dir"`
	SecretsFile       string        `yaml:"secrets_file" mapstructure:"secrets_file"`
	CAFile            string        `yaml:"ca_file" mapstructure:"ca_file"`
	HeartbeatInterval string        `yaml:"heartbeat_interval" mapstructure:"heartbeat_interval"`
	LeaseDuration     string        `yaml:"lease_duration" mapstructure:"lease_duration"`
}

// LoadAgent严格校验输入，未配置的身份不能从宿主其它客户端配置猜测。
func LoadAgent(options AgentLoadOptions) (AgentConfig, error) {
	filename, err := configurationPath(options.Filename, "agent.yml")
	if err != nil {
		return AgentConfig{}, err
	}
	data, info, err := readConfiguration(filename)
	if err != nil {
		return AgentConfig{}, invalid("Agent配置", "无法读取普通文件")
	}
	var file agentFile
	if err := decodeConfiguration(data, &file); err != nil {
		return AgentConfig{}, err
	}
	if file.Token != "" && !privateConfiguration(info) {
		return AgentConfig{}, invalid("Agent凭据", "需要自有0400或0600私有文件")
	}
	values := viper.New()
	values.SetConfigType("yaml")
	values.SetDefault("capacity", 1)
	values.SetDefault("data_dir", "~/.mybuilds/agent")
	values.SetDefault("heartbeat_interval", "5s")
	values.SetDefault("lease_duration", "30s")
	if err := values.ReadConfig(bytes.NewReader(data)); err != nil {
		return AgentConfig{}, invalid("Agent配置", "无法解码")
	}
	override, hasOverride := os.LookupEnv("MYBUILDS_AGENT_TOKEN")
	if hasOverride {
		values.Set("token", override)
	}
	var merged agentFile
	if err := values.UnmarshalExact(&merged); err != nil {
		return AgentConfig{}, invalid("Agent配置", "无法解码")
	}
	endpoint, err := configurationEndpoint(merged.Server)
	if err != nil {
		return AgentConfig{}, err
	}
	if !validAgentName(merged.Node) || merged.Capacity < 1 || merged.Capacity > 32 {
		return AgentConfig{}, invalid("Agent配置", "节点或容量不合法")
	}
	heartbeat, err := time.ParseDuration(merged.HeartbeatInterval)
	if err != nil {
		return AgentConfig{}, invalid("Agent策略", "duration不合法")
	}
	lease, err := time.ParseDuration(merged.LeaseDuration)
	if err != nil || !validLeasePolicy(heartbeat, lease) {
		return AgentConfig{}, invalid("Agent策略", "心跳或租约不合法")
	}
	token, tokenEnv, err := configurationToken(merged.Token, !hasOverride)
	if err != nil {
		return AgentConfig{}, invalid("Agent凭据", "缺少有效token")
	}
	if hasOverride {
		tokenEnv = "MYBUILDS_AGENT_TOKEN"
	}
	base := filepath.Dir(filename)
	dataDir, err := expandConfigurationPath(merged.DataDir, base)
	if err != nil {
		return AgentConfig{}, err
	}
	if info, err := os.Lstat(dataDir); err == nil {
		if !info.IsDir() || info.Mode().Perm() != 0700 || !configurationOwned(info) {
			return AgentConfig{}, invalid("Agent数据目录", "需要自有0700普通目录")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return AgentConfig{}, invalid("Agent数据目录", "无法检查目录")
	}
	secrets := ""
	if values.IsSet("secrets_file") {
		secrets, err = expandConfigurationPath(merged.SecretsFile, base)
		if err != nil {
			return AgentConfig{}, err
		}
	}
	ca := ""
	if merged.CAFile != "" {
		ca, err = expandConfigurationPath(merged.CAFile, base)
		if err != nil {
			return AgentConfig{}, err
		}
	}
	var tools *PublishTools
	if merged.PublishTools != nil {
		if merged.PublishTools.BundleDir == "" {
			return AgentConfig{}, invalid("发布工具", "缺少bundle_dir")
		}
		bundle, e := expandConfigurationPath(merged.PublishTools.BundleDir, base)
		if e != nil {
			return AgentConfig{}, e
		}
		jar := ""
		if merged.PublishTools.Bundletool != "" {
			jar, e = expandConfigurationPath(merged.PublishTools.Bundletool, base)
			if e != nil {
				return AgentConfig{}, e
			}
		}
		tools = &PublishTools{BundleDir: bundle, Bundletool: jar}
	}
	return AgentConfig{PublishTools: tools, Server: endpoint, Node: merged.Node, Capacity: merged.Capacity, DataDir: dataDir, SecretsFile: secrets, CAFile: ca, HeartbeatInterval: heartbeat, LeaseDuration: lease, RuntimeToken: token, TokenEnv: tokenEnv}, nil
}
func validAgentName(name string) bool {
	return len(name) >= 1 && len(name) <= 64 && name == strings.TrimSpace(name) && name != "." && name != ".." && !strings.ContainsAny(name, "/\\") && !containsConfigurationControl(name)
}
func validLeasePolicy(heartbeat, lease time.Duration) bool {
	return heartbeat >= time.Second && heartbeat <= 30*time.Second && lease >= 10*time.Second && lease <= 180*time.Second && lease >= 4*heartbeat+2*time.Second
}
