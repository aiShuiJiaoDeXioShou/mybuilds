package config

import (
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"
)

type DatabaseConfig struct {
	Driver string `yaml:"driver" mapstructure:"driver" json:"driver"`
	DSN    string `yaml:"dsn" mapstructure:"dsn" json:"-"`
}
type ServerConfig struct {
	Defaults          ServerDefaults          `yaml:"defaults,omitempty" mapstructure:"defaults"`
	BuildProfiles     map[string]BuildProfile `yaml:"build_profiles,omitempty" mapstructure:"build_profiles" json:"-"`
	Retention         Retention               `yaml:"retention" mapstructure:"retention" json:"retention"`
	Listen            string                  `yaml:"listen" mapstructure:"listen"`
	DataDir           string                  `yaml:"data_dir" mapstructure:"data_dir"`
	SecretsFile       string                  `yaml:"secrets_file" mapstructure:"secrets_file" json:"-"`
	Concurrency       int                     `yaml:"concurrency" mapstructure:"concurrency"`
	Database          DatabaseConfig          `yaml:"database" mapstructure:"database"`
	HeartbeatInterval time.Duration           `yaml:"heartbeat_interval" mapstructure:"heartbeat_interval"`
	LeaseDuration     time.Duration           `yaml:"lease_duration" mapstructure:"lease_duration"`
}

// serverFile保留duration的严格YAML字符串类型，合并后再转换为time.Duration。
type serverFile struct {
	Defaults          ServerDefaults          `yaml:"defaults,omitempty"`
	BuildProfiles     map[string]BuildProfile `yaml:"build_profiles,omitempty"`
	Retention         Retention               `yaml:"retention"`
	Listen            string                  `yaml:"listen"`
	DataDir           string                  `yaml:"data_dir"`
	SecretsFile       string                  `yaml:"secrets_file"`
	Concurrency       int                     `yaml:"concurrency"`
	Database          DatabaseConfig          `yaml:"database"`
	HeartbeatInterval string                  `yaml:"heartbeat_interval"`
	LeaseDuration     string                  `yaml:"lease_duration"`
}
type ServerOverrides struct {
	Listen, DataDir, SecretsFile, DatabaseDriver, DatabaseDSN *string
	Concurrency                                               *int
}
type ServerLoadOptions struct {
	Filename string
	Explicit bool
	CLI      ServerOverrides
}

// LoadServer先严格检查输入类型，再用局部Viper按明确优先级合并。
func LoadServer(options ServerLoadOptions) (ServerConfig, error) {
	filename, err := configurationPath(options.Filename, "server.yml")
	if err != nil {
		return ServerConfig{}, err
	}
	data, _, err := readConfiguration(filename)
	if err != nil && (!errors.Is(err, os.ErrNotExist) || options.Explicit) {
		return ServerConfig{}, invalid("服务端配置", "无法读取普通文件")
	}
	values := viper.New()
	values.SetConfigType("yaml")
	values.SetDefault("retention.builds", int64(100))
	values.SetDefault("retention.days", int64(30))
	values.SetDefault("listen", "127.0.0.1:8787")
	values.SetDefault("concurrency", 1)
	values.SetDefault("heartbeat_interval", "5s")
	values.SetDefault("lease_duration", "30s")
	values.SetDefault("data_dir", "~/.mybuilds")
	values.SetDefault("database.driver", "sqlite")
	values.SetDefault("secrets_file", "~/.mybuilds/secrets.env")
	if data != nil {
		var fileConfig serverFile
		if err := decodeConfiguration(data, &fileConfig); err != nil {
			return ServerConfig{}, err
		}
		if err := values.ReadConfig(bytes.NewReader(data)); err != nil {
			return ServerConfig{}, invalid("服务端配置", "无法解码")
		}
	}
	for env, key := range map[string]string{"MYBUILDS_LISTEN": "listen", "MYBUILDS_DATA_DIR": "data_dir", "MYBUILDS_DATABASE_DRIVER": "database.driver", "MYBUILDS_DATABASE_DSN": "database.dsn", "MYBUILDS_SECRETS_FILE": "secrets_file"} {
		if value, ok := os.LookupEnv(env); ok {
			if value == "" {
				return ServerConfig{}, invalid("服务端环境", "值不能为空")
			}
			values.Set(key, value)
		}
	}
	if value, ok := os.LookupEnv("MYBUILDS_CONCURRENCY"); ok {
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			return ServerConfig{}, invalid("concurrency", "需要正整数")
		}
		values.Set("concurrency", n)
	}
	for _, entry := range []struct {
		key   string
		value *string
	}{{"listen", options.CLI.Listen}, {"data_dir", options.CLI.DataDir}, {"secrets_file", options.CLI.SecretsFile}, {"database.driver", options.CLI.DatabaseDriver}, {"database.dsn", options.CLI.DatabaseDSN}} {
		if entry.value != nil {
			values.Set(entry.key, *entry.value)
		}
	}
	if options.CLI.Concurrency != nil {
		values.Set("concurrency", *options.CLI.Concurrency)
	}
	var cfg ServerConfig
	if err := values.UnmarshalExact(&cfg); err != nil {
		return cfg, invalid("服务端配置", "无法解码")
	}
	if !validLeasePolicy(cfg.HeartbeatInterval, cfg.LeaseDuration) {
		return ServerConfig{}, invalid("服务端策略", "心跳或租约不合法")
	}
	if err := validateNotifications(cfg.Defaults.Notifications, "defaults.notifications"); err != nil {
		return ServerConfig{}, err
	}
	if err := ValidateRetention(cfg.Retention); err != nil {
		return ServerConfig{}, err
	}
	host, port, err := net.SplitHostPort(cfg.Listen)
	if err != nil || !validConfigurationHost(host) {
		return ServerConfig{}, invalid("listen", "需要合法监听地址")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return ServerConfig{}, invalid("listen", "需要合法端口")
	}
	if cfg.Concurrency <= 0 {
		return ServerConfig{}, invalid("concurrency", "需要正整数")
	}
	if cfg.Database.Driver != "sqlite" && cfg.Database.Driver != "postgres" {
		return ServerConfig{}, invalid("database.driver", "仅支持sqlite/postgres")
	}
	base := filepath.Dir(filename)
	if err := validateBuildProfiles(cfg.BuildProfiles); err != nil {
		return ServerConfig{}, err
	}
	for name, profile := range cfg.BuildProfiles {
		if profile.File != "" {
			profile.File, err = expandConfigurationPath(profile.File, base)
			if err != nil {
				return ServerConfig{}, err
			}
			cfg.BuildProfiles[name] = profile
		}
	}
	cfg.DataDir, err = expandConfigurationPath(cfg.DataDir, base)
	if err != nil {
		return ServerConfig{}, err
	}
	cfg.SecretsFile, err = expandConfigurationPath(cfg.SecretsFile, base)
	if err != nil {
		return ServerConfig{}, err
	}
	if cfg.Database.Driver == "sqlite" && !values.IsSet("database.dsn") {
		cfg.Database.DSN = filepath.Join(cfg.DataDir, "mybuilds.db")
	}
	if cfg.Database.Driver == "sqlite" {
		if strings.Contains(cfg.Database.DSN, ":") || strings.ContainsAny(cfg.Database.DSN, "?\x00") {
			return ServerConfig{}, invalid("database.dsn", "只支持普通文件路径")
		}
		cfg.Database.DSN, err = expandConfigurationPath(cfg.Database.DSN, base)
		if err != nil {
			return ServerConfig{}, err
		}
	} else if strings.TrimSpace(cfg.Database.DSN) == "" || containsConfigurationControl(cfg.Database.DSN) {
		return ServerConfig{}, invalid("database.dsn", "需要合法连接配置")
	}
	return cfg, nil
}

func decodeConfiguration(data []byte, target any) error {
	if len(data) > MaxConfigBytes {
		return invalid("配置", "超过大小上限")
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var node yaml.Node
	if err := dec.Decode(&node); err != nil {
		return invalid("配置", "YAML格式错误")
	}
	count := 0
	if err := checkTree(&node, 0, &count); err != nil {
		return err
	}
	if len(node.Content) != 1 || node.Content[0].Kind != yaml.MappingNode {
		return invalid("配置", "需要映射")
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err != io.EOF {
		return invalid("配置", "只允许一个YAML文档")
	}
	if err := checkType(node.Content[0], reflect.TypeOf(target).Elem(), "配置"); err != nil {
		return err
	}
	if err := node.Content[0].Decode(target); err != nil {
		return invalid("配置", "无法解码")
	}
	return nil
}

// readConfiguration为实际管理配置消费者提供有限普通文件读取。
func readConfiguration(filename string) ([]byte, os.FileInfo, error) {
	file, err := openConfiguration(filename)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, nil, os.ErrPermission
	}
	if info.Size() > MaxConfigBytes {
		return nil, nil, invalid("配置", "超过大小上限")
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxConfigBytes+1))
	if err != nil {
		return nil, nil, os.ErrPermission
	}
	if len(data) > MaxConfigBytes {
		return nil, nil, invalid("配置", "超过大小上限")
	}
	return data, info, nil
}
func configurationPath(filename, name string) (string, error) {
	if filename == "" {
		filename = filepath.Join("~/.mybuilds", name)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", invalid("配置路径", "无法确定目录")
	}
	return expandConfigurationPath(filename, cwd)
}
func containsConfigurationControl(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}
func expandConfigurationPath(value, base string) (string, error) {
	if strings.TrimSpace(value) == "" || containsConfigurationControl(value) {
		return "", invalid("配置路径", "路径不合法")
	}
	if value == "~" || strings.HasPrefix(value, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", invalid("配置路径", "无法展开用户目录")
		}
		if value == "~" {
			value = home
		} else {
			value = filepath.Join(home, strings.TrimPrefix(value, "~/"))
		}
	} else if strings.HasPrefix(value, "~") {
		return "", invalid("配置路径", "不支持其他用户目录")
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(base, value)
	}
	return filepath.Clean(value), nil
}

// validConfigurationHost只检查语法，不发起DNS或其他网络查询。
func validConfigurationHost(host string) bool {
	if host == "" || net.ParseIP(host) != nil {
		return true
	}
	if len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(strings.TrimSuffix(host, "."), ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}
