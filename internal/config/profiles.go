package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"
)

type BuildProfile struct {
	Template string `yaml:"template,omitempty" mapstructure:"template"`
	File     string `yaml:"file,omitempty" mapstructure:"file" json:"-"`
}

type LoadedProfile struct {
	Name, Template, ContentDigest string
	Definition                    Build
	Notifications                 *Notifications
	Content                       []byte `json:"-"`
}

var builtinProfileNames = []string{"native-android", "native-ios", "flutter-android", "flutter-ios"}

func validProfileName(name string) bool { return len(name) <= 64 && safeName(name) }
func builtinProfileName(name string) bool {
	for _, n := range builtinProfileNames {
		if n == name {
			return true
		}
	}
	return false
}
func validateBuildProfiles(entries map[string]BuildProfile) error {
	if len(entries) > 64 {
		return invalid("build_profiles", "超过数量上限")
	}
	for _, name := range sortedKeys(entries) {
		p := entries[name]
		if !validProfileName(name) || builtinProfileName(name) {
			return invalid("build_profiles", "方案名称不合法或保留")
		}
		if (p.File == "") == (p.Template == "") {
			return invalid("build_profiles", "template/file必须二选一")
		}
		if p.Template != "" && !builtinProfileName(p.Template) {
			return invalid("build_profiles.template", "内置方案不存在")
		}
		if p.File != "" && containsConfigurationControl(p.File) {
			return invalid("build_profiles.file", "路径不合法")
		}
	}
	return nil
}

// ParseBuildProfile复用流水线树与类型检查，只拒绝方案不允许的builds根键。
func ParseBuildProfile(data []byte) (*Build, *Notifications, error) {
	doc, err := Parse(data)
	if err != nil {
		return nil, nil, err
	}
	var node yaml.Node
	if err := yaml.NewDecoder(bytes.NewReader(data)).Decode(&node); err != nil {
		return nil, nil, invalid("方案", "无法解码")
	}
	if mapping(node.Content[0])["builds"] != nil {
		return nil, nil, invalid("方案", "不允许嵌套builds")
	}
	return doc.Builds["default"], doc.Notifications, nil
}

// LoadBuildProfiles只在控制端启动消费管理员明确文件；坏项整批拒绝。
func LoadBuildProfiles(entries map[string]BuildProfile, builtins map[string][]byte) (map[string]LoadedProfile, error) {
	if err := validateBuildProfiles(entries); err != nil {
		return nil, err
	}
	result := make(map[string]LoadedProfile, len(entries)+len(builtins))
	total := 0
	for _, name := range sortedKeys(builtins) {
		if !builtinProfileName(name) {
			return nil, invalid("内置方案", "名称不合法")
		}
		data := builtins[name]
		total += len(data)
		if total > 16*MaxConfigBytes {
			return nil, invalid("方案", "超过集合大小上限")
		}
		doc, err := Parse(data)
		if err != nil {
			return nil, err
		}
		platform := name[strings.LastIndex(name, "-")+1:]
		b := doc.Builds[platform]
		// 兼容同一配置语义的根级内置模板，但不能忽略额外build。
		if b == nil {
			b = doc.Builds["default"]
		}
		if b == nil || len(doc.Builds) != 1 {
			return nil, invalid("内置方案", "必须只有对应平台构建")
		}
		result[name] = loadedProfile(name, name, data, b, doc.Notifications)
	}
	for _, name := range sortedKeys(entries) {
		p := entries[name]
		if p.Template != "" {
			original, ok := result[p.Template]
			if !ok {
				return nil, invalid("build_profiles.template", "内置方案尚未交付")
			}
			copy, err := CopyLoadedProfile(original)
			if err != nil {
				return nil, err
			}
			copy.Name = name
			result[name] = copy
			total += len(original.Content)
		} else {
			data, err := readProfileFile(p.File)
			if err != nil {
				return nil, invalid("方案文件", "无法读取完整普通文件")
			}
			total += len(data)
			b, n, err := ParseBuildProfile(data)
			if err != nil {
				return nil, err
			}
			result[name] = loadedProfile(name, "", data, b, n)
		}
		if total > 16*MaxConfigBytes {
			return nil, invalid("方案", "超过集合大小上限")
		}
	}
	return result, nil
}
func loadedProfile(name, template string, data []byte, b *Build, n *Notifications) LoadedProfile {
	sum := sha256.Sum256(data)
	return LoadedProfile{Name: name, Template: template, ContentDigest: hex.EncodeToString(sum[:]), Definition: *b, Notifications: n, Content: bytes.Clone(data)}
}

// CopyLoadedProfile给每次项目展开独立map/slice；不修改启动时冻结的方案。
func CopyLoadedProfile(in LoadedProfile) (LoadedProfile, error) {
	data, err := json.Marshal(in)
	if err != nil {
		return LoadedProfile{}, invalid("方案", "无法复制")
	}
	var out LoadedProfile
	if err := json.Unmarshal(data, &out); err != nil {
		return LoadedProfile{}, invalid("方案", "无法复制")
	}
	out.Content = bytes.Clone(in.Content)
	return out, nil
}
func readProfileFile(filename string) ([]byte, error) {
	before, err := os.Lstat(filename)
	if err != nil || !before.Mode().IsRegular() {
		return nil, os.ErrPermission
	}
	f, err := openConfiguration(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(before, info) || !profileSingleLink(f, info) || info.Size() > MaxConfigBytes {
		return nil, os.ErrPermission
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxConfigBytes+1))
	if err != nil || len(data) > MaxConfigBytes {
		return nil, os.ErrPermission
	}
	after, e := f.Stat()
	current, e2 := os.Lstat(filename)
	if e != nil || e2 != nil || !current.Mode().IsRegular() || !os.SameFile(info, after) || !os.SameFile(info, current) || info.Size() != after.Size() || int64(len(data)) != after.Size() || !info.ModTime().Equal(after.ModTime()) || !profileSingleLink(f, after) {
		return nil, os.ErrPermission
	}
	return data, nil
}
