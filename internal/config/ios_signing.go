package config

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// IOSSigning 只保存明确引用与原字段，不在配置阶段读取密钥或创建资源。
type IOSSigning struct {
	P12          string `yaml:"p12"`
	Profile      string `yaml:"profile"`
	Password     string `yaml:"password"`
	BundleID     string `yaml:"bundle_id"`
	ExportMethod string `yaml:"export_method"`
}

var iosBundleID = regexp.MustCompile(`^[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+$`)

// ValidateIOSSigning 检查定义或一次渲染后的值，不读取环境或材料。
func ValidateIOSSigning(signing *IOSSigning, field string) error {
	if signing == nil {
		return nil
	}
	for _, item := range []struct{ name, value string }{{"p12", signing.P12}, {"profile", signing.Profile}, {"password", signing.Password}} {
		if !secretReference.MatchString(item.value) {
			return invalid(field+"."+item.name, "需要完整环境引用")
		}
	}
	for _, item := range []struct{ name, value string }{{"bundle_id", signing.BundleID}, {"export_method", signing.ExportMethod}} {
		if strings.TrimSpace(item.value) == "" || strings.ContainsFunc(item.value, unicode.IsControl) {
			return invalid(field+"."+item.name, "需要非空安全值")
		}
		// 只检查原文模板格式，变量存在性和一次渲染结果由预览/执行阶段校验。
		template := false
		literal := fieldTemplate.ReplaceAllStringFunc(item.value, func(match string) string {
			template = true
			if strings.TrimSpace(match[2:len(match)-2]) == "" {
				return "{}"
			}
			return "template"
		})
		if strings.ContainsAny(literal, "{}") {
			return invalid(field+"."+item.name, "模板格式错误")
		}
		if template {
			continue
		}
		if item.name == "bundle_id" && (!iosBundleID.MatchString(item.value) || len(item.value) > 255) {
			return invalid(field+"."+item.name, "需要安全反向域名标识")
		}
		if item.name == "export_method" && !slices.Contains([]string{"debugging", "release-testing", "app-store-connect", "enterprise"}, item.value) {
			return invalid(field+"."+item.name, "不支持的导出方式")
		}
	}
	return nil
}
