package config

import (
	"bytes"
	"errors"
	"github.com/spf13/viper"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type ClientConfig struct {
	Server       string
	Timeout      time.Duration
	RuntimeToken string `json:"-"`
	CAFile       string
}
type ClientLoadOptions struct {
	Filename  string
	Explicit  bool
	ServerURL *string
	Timeout   *time.Duration
	CAFile    *string
}

// LoadClient只由远程命令调用，不影响本地run、doctor或init。
func LoadClient(options ClientLoadOptions) (ClientConfig, error) {
	filename, err := configurationPath(options.Filename, "client.yml")
	if err != nil {
		return ClientConfig{}, err
	}
	data, info, err := readConfiguration(filename)
	if err != nil && (!errors.Is(err, os.ErrNotExist) || options.Explicit) {
		return ClientConfig{}, invalid("客户端配置", "无法读取普通文件")
	}
	type fileClient struct {
		Server  string `yaml:"server" mapstructure:"server"`
		Token   string `yaml:"token" mapstructure:"token"`
		Timeout string `yaml:"timeout" mapstructure:"timeout"`
		CAFile  string `yaml:"ca_file" mapstructure:"ca_file"`
	}
	values := viper.New()
	values.SetConfigType("yaml")
	values.SetDefault("server", "http://127.0.0.1:8787")
	values.SetDefault("timeout", "30s")
	if data != nil {
		var file fileClient
		if err := decodeConfiguration(data, &file); err != nil {
			return ClientConfig{}, err
		}
		if file.Token != "" && !privateConfiguration(info) {
			return ClientConfig{}, invalid("客户端凭据", "需要自有0400或0600私有文件")
		}
		if err := values.ReadConfig(bytes.NewReader(data)); err != nil {
			return ClientConfig{}, invalid("客户端配置", "无法解码")
		}
	}
	for env, key := range map[string]string{"MYBUILDS_SERVER_URL": "server", "MYBUILDS_CLIENT_TOKEN": "token", "MYBUILDS_CLIENT_TIMEOUT": "timeout", "MYBUILDS_CA_FILE": "ca_file"} {
		if value, ok := os.LookupEnv(env); ok {
			values.Set(key, value)
		}
	}
	if options.ServerURL != nil {
		values.Set("server", *options.ServerURL)
	}
	if options.Timeout != nil {
		values.Set("timeout", options.Timeout.String())
	}
	if options.CAFile != nil {
		values.Set("ca_file", *options.CAFile)
	}
	var merged fileClient
	if err := values.UnmarshalExact(&merged); err != nil {
		return ClientConfig{}, invalid("客户端配置", "无法解码")
	}
	endpoint, err := configurationEndpoint(merged.Server)
	if err != nil {
		return ClientConfig{}, err
	}
	timeout, err := time.ParseDuration(merged.Timeout)
	if err != nil || timeout <= 0 {
		return ClientConfig{}, invalid("timeout", "需要正duration")
	}
	_, override := os.LookupEnv("MYBUILDS_CLIENT_TOKEN")
	token, _, err := configurationToken(merged.Token, !override)
	if err != nil {
		return ClientConfig{}, err
	}
	ca := ""
	if merged.CAFile != "" {
		ca, err = expandConfigurationPath(merged.CAFile, filepath.Dir(filename))
		if err != nil {
			return ClientConfig{}, err
		}
	}
	return ClientConfig{Server: endpoint, Timeout: timeout, RuntimeToken: token, CAFile: ca}, nil
}

// configurationEndpoint在Agent与远程客户端之间复用URL边界。
func configurationEndpoint(value string) (string, error) {
	endpoint, err := url.Parse(value)
	if err != nil || endpoint.Host == "" || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.ForceQuery || endpoint.Fragment != "" || endpoint.Opaque != "" || !validConfigurationHost(endpoint.Hostname()) {
		return "", invalid("server", "需要合法控制端URL")
	}
	if port := endpoint.Port(); port != "" {
		n, e := strconv.Atoi(port)
		if e != nil || n < 1 || n > 65535 {
			return "", invalid("server", "端口不合法")
		}
	}
	ip := net.ParseIP(endpoint.Hostname())
	if endpoint.Scheme != "https" && !(endpoint.Scheme == "http" && (endpoint.Hostname() == "localhost" || ip != nil && ip.IsLoopback())) {
		return "", invalid("server", "需要HTTPS或本机HTTP")
	}
	return strings.TrimRight(endpoint.String(), "/"), nil
}

// configurationToken只解释原配置的一次完整引用；宿主插入值保持原文。
func configurationToken(value string, resolve bool) (string, string, error) {
	token := value
	tokenEnv := ""
	// 引用仅解释一次；宿主密钥插入的内容不再次读取或插值。
	if resolve && strings.HasPrefix(token, "${") && strings.HasSuffix(token, "}") {
		name := strings.TrimSuffix(strings.TrimPrefix(token, "${"), "}")
		if !identifier.MatchString(name) {
			return "", "", invalid("客户端凭据", "引用不合法")
		}
		token = os.Getenv(name)
		tokenEnv = name
	}
	if len(token) < 32 || len(token) > 4096 {
		return "", "", invalid("客户端凭据", "缺少有效token")
	}
	for _, r := range token {
		if r < 33 || r > 126 {
			return "", "", invalid("客户端凭据", "token不合法")
		}
	}
	return token, tokenEnv, nil
}
