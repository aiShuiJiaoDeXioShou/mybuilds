package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

var yamlLine = regexp.MustCompile(`^yaml: line ([0-9]+):`)

// Load 限制读取大小；诊断不包含文件名或底层错误中的敏感内容。
func Load(filename string) (*Document, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("配置文件：无法读取")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxConfigBytes+1))
	if err != nil {
		return nil, fmt.Errorf("配置文件：读取失败")
	}
	return Parse(data)
}

// Parse 先检查 YAML 节点的边界、精确类型和字段，再解码及规范化。
func Parse(data []byte) (*Document, error) {
	if len(data) > MaxConfigBytes {
		return nil, fmt.Errorf("配置：超过大小上限")
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var node yaml.Node
	if err := dec.Decode(&node); err != nil {
		return nil, syntaxError(err)
	}
	count := 0
	if err := checkTree(&node, 0, &count); err != nil {
		return nil, err
	}
	if len(node.Content) != 1 || node.Content[0].Kind != yaml.MappingNode {
		return nil, nodeError(&node, "配置", "需要非空映射")
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, nodeError(&extra, "配置", "只允许一个 YAML 文档")
	}
	root := node.Content[0]
	fields := mapping(root)
	multi := fields["builds"] != nil
	buildType := reflect.TypeOf(Build{})
	allowed := structFields(buildType)
	allowed["version"] = reflect.TypeOf(int(0))
	allowed["builds"] = reflect.TypeOf(map[string]*Build{})
	if multi {
		allowed = map[string]reflect.Type{"version": reflect.TypeOf(int(0)), "builds": reflect.TypeOf(map[string]*Build{}), "notifications": reflect.TypeOf((*Notifications)(nil))}
	}
	for i := 0; i < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		typ, ok := allowed[key.Value]
		if !ok {
			return nil, nodeError(key, "配置", "未知字段或混写单/多构建格式")
		}
		if err := checkType(value, typ, key.Value); err != nil {
			return nil, err
		}
	}
	version := fields["version"]
	if version == nil {
		return nil, nodeError(root, "version", "缺少必要字段")
	}
	var v int
	if err := version.Decode(&v); err != nil {
		return nil, nodeError(version, "version", "无效整数")
	}
	d := &Document{Version: v, Builds: make(map[string]*Build)}
	if multi {
		builds := fields["builds"]
		for i := 0; i < len(builds.Content); i += 2 {
			b, err := decodeBuild(builds.Content[i+1])
			if err != nil {
				return nil, err
			}
			d.Builds[builds.Content[i].Value] = b
		}
		if n := fields["notifications"]; n != nil {
			var notifications Notifications
			if err := n.Decode(&notifications); err != nil {
				return nil, nodeError(n, "notifications", "无法解码")
			}
			d.Notifications = &notifications
		}
	} else {
		// 根级 version 不属于 Build，复用节点而不重复解析原字节。
		legacy := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Line: root.Line}
		for i := 0; i < len(root.Content); i += 2 {
			if root.Content[i].Value != "version" {
				legacy.Content = append(legacy.Content, root.Content[i], root.Content[i+1])
			}
		}
		b, err := decodeBuild(legacy)
		if err != nil {
			return nil, err
		}
		d.Builds["default"] = b
	}
	if err := Validate(d); err != nil {
		return nil, err
	}
	return d, nil
}

func syntaxError(err error) error {
	if match := yamlLine.FindStringSubmatch(err.Error()); len(match) == 2 {
		return fmt.Errorf("配置第 %s 行：无效 YAML", match[1])
	}
	return fmt.Errorf("配置：无效或空 YAML")
}

func nodeError(n *yaml.Node, field, reason string) error {
	if n != nil && n.Line > 0 {
		return fmt.Errorf("%s（第 %d 行，第 %d 列）：%s", field, n.Line, n.Column, reason)
	}
	return fmt.Errorf("%s：%s", field, reason)
}

func checkTree(n *yaml.Node, depth int, count *int) error {
	*count++
	if *count > MaxNodes {
		return nodeError(n, "配置", "超过节点上限")
	}
	if depth > MaxDepth {
		return nodeError(n, "配置", "超过嵌套上限")
	}
	if n.Kind == yaml.AliasNode {
		return nodeError(n, "配置", "不允许 YAML 别名")
	}
	if n.Tag == "!!null" {
		return nodeError(n, "配置", "不允许 null")
	}
	if n.Kind == yaml.MappingNode {
		seen := make(map[string]bool)
		for i := 0; i < len(n.Content); i += 2 {
			key := n.Content[i]
			if key.Tag == "!!merge" || key.Value == "<<" {
				return nodeError(key, "配置", "不允许合并映射")
			}
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
				return nodeError(key, "配置", "映射键必须是字符串")
			}
			if seen[key.Value] {
				return nodeError(key, "配置", "重复映射键")
			}
			seen[key.Value] = true
		}
	}
	for _, child := range n.Content {
		if err := checkTree(child, depth+1, count); err != nil {
			return err
		}
	}
	return nil
}

func mapping(n *yaml.Node) map[string]*yaml.Node {
	values := make(map[string]*yaml.Node)
	for i := 0; i < len(n.Content); i += 2 {
		values[n.Content[i].Value] = n.Content[i+1]
	}
	return values
}

func structFields(t reflect.Type) map[string]reflect.Type {
	fields := make(map[string]reflect.Type)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name := strings.Split(field.Tag.Get("yaml"), ",")[0]
		if name != "" {
			fields[name] = field.Type
		}
	}
	return fields
}

func checkType(n *yaml.Node, t reflect.Type, field string) error {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	// 参数字符串简写仅在 Parameter 处允许；其他字符串不接受数值转换。
	if t == reflect.TypeOf(Parameter{}) && n.Kind == yaml.ScalarNode && n.Tag == "!!str" {
		return nil
	}
	switch t.Kind() {
	case reflect.String, reflect.Bool, reflect.Int, reflect.Int64:
		tag := map[reflect.Kind]string{reflect.String: "!!str", reflect.Bool: "!!bool", reflect.Int: "!!int", reflect.Int64: "!!int"}[t.Kind()]
		if n.Kind != yaml.ScalarNode || n.Tag != tag {
			return nodeError(n, field, "标量类型不正确")
		}
	case reflect.Slice:
		if n.Kind != yaml.SequenceNode {
			return nodeError(n, field, "需要列表")
		}
		for i, item := range n.Content {
			if err := checkType(item, t.Elem(), fmt.Sprintf("%s[%d]", field, i+1)); err != nil {
				return err
			}
		}
	case reflect.Map:
		if n.Kind != yaml.MappingNode {
			return nodeError(n, field, "需要映射")
		}
		for i := 0; i < len(n.Content); i += 2 {
			if err := checkType(n.Content[i+1], t.Elem(), fmt.Sprintf("%s[%d]", field, i/2+1)); err != nil {
				return err
			}
		}
	case reflect.Struct:
		if n.Kind != yaml.MappingNode {
			return nodeError(n, field, "需要映射")
		}
		fields := structFields(t)
		if t == reflect.TypeOf(Step{}) {
			values := mapping(n)
			kind := values["kind"]
			if kind == nil {
				return nodeError(n, field+".kind", "缺少必要字段")
			}
			if err := checkType(kind, reflect.TypeOf(""), field+".kind"); err != nil {
				return err
			}
			target := ""
			if value := values["target"]; value != nil {
				target = value.Value
			}
			permitted := stepFields(kind.Value, target)
			for key := range fields {
				if !permitted[key] {
					delete(fields, key)
				}
			}
		}
		for i := 0; i < len(n.Content); i += 2 {
			key, value := n.Content[i], n.Content[i+1]
			typ, ok := fields[key.Value]
			if !ok {
				return nodeError(key, field, "未知或不适用字段")
			}
			path := field + "." + key.Value
			if err := checkType(value, typ, path); err != nil {
				return err
			}
			if err := checkPresent(value, key.Value, path); err != nil {
				return err
			}
		}
	default:
		return nodeError(n, field, "不支持的结构类型")
	}
	return nil
}

// 检查显式空字段，避免解码后的零值把空值与未填写混淆。
func checkPresent(n *yaml.Node, key, field string) error {
	switch key {
	case "name", "shell", "working_dir", "timeout", "framework", "platform", "target", "file", "credentials", "url", "result_file", "app_identifier", "channel", "track", "release_status":
		if n.Kind == yaml.ScalarNode && strings.TrimSpace(n.Value) == "" {
			return nodeError(n, field, "不能为空")
		}
	case "choices", "branches", "changes", "paths", "argv", "query_argv", "on":
		if len(n.Content) == 0 {
			return nodeError(n, field, "列表不能为空")
		}
	case "when", "post", "reports", "junit", "ios_signing":
		if len(n.Content) == 0 {
			return nodeError(n, field, "映射不能为空")
		}
	}
	if strings.HasSuffix(field, "when.params") && len(n.Content) == 0 {
		return nodeError(n, field, "条件映射不能为空")
	}
	return nil
}

func decodeBuild(n *yaml.Node) (*Build, error) {
	// 将参数简写变成 default 字段，随后全部复用标准结构解码。
	if params := mapping(n)["params"]; params != nil {
		for i := 1; i < len(params.Content); i += 2 {
			value := params.Content[i]
			if value.Kind == yaml.ScalarNode {
				params.Content[i] = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: "default"}, value}}
			}
		}
	}
	// post 内的 run/artifact 也会通过 checkType；根级 Build 检查包含显式空字段。
	if err := checkType(n, reflect.TypeOf(Build{}), "builds"); err != nil {
		return nil, err
	}
	var build Build
	if err := n.Decode(&build); err != nil {
		return nil, nodeError(n, "builds", "无法解码配置")
	}
	return &build, nil
}

func indexed(field string, index int) string { return field + "[" + strconv.Itoa(index+1) + "]" }
