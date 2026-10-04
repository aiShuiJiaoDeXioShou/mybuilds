package config

import (
	"bytes"
	"errors"
	"github.com/spf13/viper"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type ClientConfig struct {
	Server       string
	Timeout      time.Duration
	RuntimeToken string `json:"-"`
}
type ClientLoadOptions struct {
	Filename  string
	Explicit  bool
	ServerURL *string
	Timeout   *time.Duration
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
		if file.Token != "" && info.Mode().Perm() != 0600 {
			return ClientConfig{}, invalid("客户端凭据", "需要0600私有文件")
		}
		if err := values.ReadConfig(bytes.NewReader(data)); err != nil {
			return ClientConfig{}, invalid("客户端配置", "无法解码")
		}
	}
	for env, key := range map[string]string{"MYBUILDS_SERVER_URL": "server", "MYBUILDS_CLIENT_TOKEN": "token", "MYBUILDS_CLIENT_TIMEOUT": "timeout"} {
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
	var merged fileClient
	if err := values.UnmarshalExact(&merged); err != nil {
		return ClientConfig{}, invalid("客户端配置", "无法解码")
	}
	endpoint, err := url.Parse(merged.Server)
	if err != nil || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.ForceQuery || endpoint.Fragment != "" || endpoint.Opaque != "" || !validConfigurationHost(endpoint.Hostname()) {
		return ClientConfig{}, invalid("server", "需要合法控制端URL")
	}
	if port := endpoint.Port(); port != "" {
		n, e := strconv.Atoi(port)
		if e != nil || n < 1 || n > 65535 {
			return ClientConfig{}, invalid("server", "端口不合法")
		}
	}
	ip := net.ParseIP(endpoint.Hostname())
	if endpoint.Scheme != "https" && !(endpoint.Scheme == "http" && (endpoint.Hostname() == "localhost" || ip != nil && ip.IsLoopback())) {
		return ClientConfig{}, invalid("server", "需要HTTPS或本机HTTP")
	}
	timeout, err := time.ParseDuration(merged.Timeout)
	if err != nil || timeout <= 0 {
		return ClientConfig{}, invalid("timeout", "需要正duration")
	}
	token := merged.Token
	// 引用仅解释一次；宿主密钥插入的内容不再次读取或插值。
	if strings.HasPrefix(token, "${") && strings.HasSuffix(token, "}") {
		name := strings.TrimSuffix(strings.TrimPrefix(token, "${"), "}")
		if !identifier.MatchString(name) {
			return ClientConfig{}, invalid("客户端凭据", "引用不合法")
		}
		token = os.Getenv(name)
	}
	if len(token) < 32 || len(token) > 4096 {
		return ClientConfig{}, invalid("客户端凭据", "缺少有效token")
	}
	for _, r := range token {
		if r < 33 || r > 126 {
			return ClientConfig{}, invalid("客户端凭据", "token不合法")
		}
	}
	return ClientConfig{Server: strings.TrimRight(endpoint.String(), "/"), Timeout: timeout, RuntimeToken: token}, nil
}
