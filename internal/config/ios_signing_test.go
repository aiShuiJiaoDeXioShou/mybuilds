package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const iosSigningConfiguration = `ios_signing:
  p12: "${IOS_CONFIG_TEST_P12}"
  profile: "${IOS_CONFIG_TEST_PROFILE}"
  password: "${IOS_CONFIG_TEST_PASSWORD}"
  bundle_id: com.example.mybuilds
  export_method: debugging
steps: [{kind: run, run: 'true'}]
`

func signingYAML(content string, multi bool) string {
	if !multi {
		return "version: 1\n" + content
	}
	return "version: 1\nbuilds:\n  ios:\n    " + strings.ReplaceAll(strings.TrimSuffix(content, "\n"), "\n", "\n    ") + "\n"
}

func TestIOSSigningLegacyAndNamedBuildDoNotResolveSecrets(t *testing.T) {
	t.Setenv("IOS_CONFIG_TEST_P12", "/PRIVATE_P12_FILE")
	t.Setenv("IOS_CONFIG_TEST_PROFILE", "/PRIVATE_PROFILE_FILE")
	t.Setenv("IOS_CONFIG_TEST_PASSWORD", "PRIVATE_PASSWORD_VALUE")
	for _, multi := range []bool{false, true} {
		t.Run(fmt.Sprint(multi), func(t *testing.T) {
			document, err := Parse([]byte(signingYAML(iosSigningConfiguration, multi)))
			if err != nil {
				t.Fatalf("合法签名定义不能解析: %v", err)
			}
			before, _ := json.Marshal(document)
			if err := Validate(document); err != nil {
				t.Fatal(err)
			}
			after, _ := json.Marshal(document)
			if string(before) != string(after) || strings.Contains(string(after), "PRIVATE_") || !strings.Contains(string(after), "${IOS_CONFIG_TEST_PASSWORD}") {
				t.Fatal("配置校验读取或改写了秘密引用")
			}
		})
	}
}

func TestIOSSigningAcceptsMethodsAndSinglePassTemplates(t *testing.T) {
	for _, method := range []string{"debugging", "release-testing", "app-store-connect", "enterprise", "{{export_method}}"} {
		input := strings.Replace(iosSigningConfiguration, "export_method: debugging", "export_method: '"+method+"'", 1)
		input = "params: {bundle_id: com.example.app, suffix: app, export_method: debugging}\n" + input
		for _, bundle := range []string{"com.example.app", "com.example.my-app", "{{bundle_id}}", "com.example.{{suffix}}", "{{ git.branch }}.{{suffix}}"} {
			inputWithBundle := strings.Replace(input, "bundle_id: com.example.mybuilds", "bundle_id: '"+bundle+"'", 1)
			if _, err := Parse([]byte(signingYAML(inputWithBundle, true))); err != nil {
				t.Fatalf("合法标识或模板被拒绝: %v", err)
			}
		}
	}
}

func TestIOSSigningRejectsInvalidSchemaAndValuesSafely(t *testing.T) {
	invalid := []string{
		"ios_signing: {}\nsteps: [{kind: run, run: 'true'}]\n",
		"ios_signing: null\nsteps: [{kind: run, run: 'true'}]\n",
		"ios_signing: []\nsteps: [{kind: run, run: 'true'}]\n",
	}
	for _, field := range []string{"p12", "profile", "password", "bundle_id", "export_method"} {
		for _, value := range []string{"''", "' '", "null", "true", "42", "[]", "{}"} {
			lines := strings.Split(iosSigningConfiguration, "\n")
			for i, line := range lines {
				if strings.HasPrefix(line, "  "+field+":") {
					lines[i] = "  " + field + ": " + value
				}
			}
			invalid = append(invalid, strings.Join(lines, "\n"))
		}
		lines := strings.Split(iosSigningConfiguration, "\n")
		for i, line := range lines {
			if strings.HasPrefix(line, "  "+field+":") {
				lines = append(lines[:i], lines[i+1:]...)
				break
			}
		}
		invalid = append(invalid, strings.Join(lines, "\n"))
	}
	invalid = append(invalid, strings.Replace(iosSigningConfiguration, "ios_signing:", "ios_signing:\n  PRIVATE_UNKNOWN: secret", 1))
	for _, field := range []string{"p12", "profile", "password"} {
		for _, value := range []string{"PRIVATE_LITERAL", "${PRIVATE-BAD}", "${}", "prefix${PRIVATE_ENV}", "${PRIVATE_ENV}suffix", "${PRIVATE_ONE}${PRIVATE_TWO}", "{{PRIVATE_PARAM}}"} {
			prefix := "  " + field + ": "
			lines := strings.Split(iosSigningConfiguration, "\n")
			for i, line := range lines {
				if strings.HasPrefix(line, prefix) {
					lines[i] = prefix + "'" + value + "'"
				}
			}
			invalid = append(invalid, strings.Join(lines, "\n"))
		}
	}
	for _, bundle := range []string{"PRIVATE_SINGLE", "com..PRIVATE", "com.PRIVATE_*", "com/PRIVATE/app", ".com.PRIVATE", "com.PRIVATE.", strings.Repeat("a", 254) + ".b", "com.{{PRIVATE", "com.PRIVATE}}", "{{ }}", "{{{PRIVATE}}}"} {
		invalid = append(invalid, strings.Replace(iosSigningConfiguration, "bundle_id: com.example.mybuilds", "bundle_id: '"+bundle+"'", 1))
	}
	for _, method := range []string{"PRIVATE_METHOD", "app-store", "ad-hoc", "development", "{{PRIVATE", "PRIVATE}}", "{{ }}", "{{{PRIVATE}}}"} {
		invalid = append(invalid, strings.Replace(iosSigningConfiguration, "export_method: debugging", "export_method: '"+method+"'", 1))
	}
	for i, input := range invalid {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			for _, multi := range []bool{false, true} {
				_, err := Parse([]byte(signingYAML(input, multi)))
				if err == nil || strings.Contains(err.Error(), "PRIVATE") {
					t.Fatalf("无效签名输入被接受或泄露: %v", err)
				}
			}
		})
	}
}

// 直接构造对象也必须经过同一规则，不能只依赖 YAML 的结构检查。
func TestIOSSigningValidateProgrammaticAndParameterRules(t *testing.T) {
	valid := &IOSSigning{P12: "${IOS_P12}", Profile: "${IOS_PROFILE}", Password: "${IOS_PASSWORD}", BundleID: "{{bundle_id}}", ExportMethod: "{{method}}"}
	defaultMethod := "debugging"
	doc := &Document{Version: 1, Builds: map[string]*Build{"ios": {IOSSigning: valid, Params: map[string]Parameter{"bundle_id": {Required: true}, "method": {Default: &defaultMethod, Choices: []string{"debugging", "enterprise"}}}, Steps: []Step{{Kind: "run", Run: "true"}}}}}
	if err := Validate(doc); err != nil {
		t.Fatal(err)
	}
	selected, err := doc.Select([]string{"ios"}, false)
	if err != nil || len(selected) != 1 || selected[0] != "ios" {
		t.Fatalf("构建选择失效: %v", err)
	}
	build := doc.Builds[selected[0]]
	if _, err := ResolveParams(build, nil); err == nil {
		t.Fatal("缺失必填参数被接受")
	}
	values, err := ResolveParams(build, map[string]string{"bundle_id": "com.example.app"})
	if err != nil || values["method"] != "debugging" {
		t.Fatalf("默认参数失效: %v", err)
	}
	if _, err := ResolveParams(build, map[string]string{"bundle_id": "com.example.app", "method": "PRIVATE_BAD"}); err == nil || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal("选项约束失效或回显")
	}
	valid.Password = "PRIVATE_LITERAL"
	if err := Validate(doc); err == nil || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal("直接构造的literal密码被接受或回显")
	}
}

func TestIOSSigningDuplicateAndAliasRejected(t *testing.T) {
	invalid := []string{
		strings.Replace(iosSigningConfiguration, "  password:", "  p12: '${OTHER_PRIVATE}'\n  password:", 1),
		strings.Replace(iosSigningConfiguration, "ios_signing:", "ios_signing: &PRIVATE_ALIAS", 1) + "post: {always: [{kind: run, run: 'true', env: {A: *PRIVATE_ALIAS}}]}\n",
		strings.Replace(iosSigningConfiguration, "ios_signing:", "ios_signing:\n  <<: {PRIVATE: secret}", 1),
	}
	for _, value := range invalid {
		if _, err := Parse([]byte(signingYAML(value, true))); err == nil || strings.Contains(err.Error(), "PRIVATE") {
			t.Fatal("重复、别名或merge被接受或回显")
		}
	}
}

func TestIOSSigningNilPreservesLegacyJSONShape(t *testing.T) {
	document, err := Parse([]byte("version: 1\nsteps: [{kind: run, run: 'true'}]\n"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "IOSSigning") {
		t.Fatal("nil签名定义改变原非IOS快照wire")
	}
}
