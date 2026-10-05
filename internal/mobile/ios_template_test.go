package mobile

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"mybuilds/internal/config"
)

func iosTemplateRuns(t *testing.T) []string {
	t.Helper()
	var document struct {
		Builds map[string]struct{ Steps []struct{ Kind, Run string } }
	}
	if yaml.Unmarshal(IOSTemplate(), &document) != nil {
		t.Fatal("模板YAML非法")
	}
	runs := []string{}
	for _, step := range document.Builds["ios"].Steps {
		if step.Kind == "run" {
			runs = append(runs, step.Run)
		}
	}
	if len(runs) != 2 {
		t.Fatal("原生模板需archive/export两个run")
	}
	return runs
}

func TestIOSTemplateShellAndVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Bash模板在Unix验证")
	}
	root := t.TempDir()
	runs := iosTemplateRuns(t)
	for _, script := range runs {
		if _, err := toolOutput(context.Background(), toolCommand{Workspace: root, Executable: "bash", Args: []string{"-n"}, Stdin: []byte(script)}); err != nil {
			t.Fatal("模板Bash语法非法")
		}
	}
	prefix := runs[0][:strings.Index(runs[0], "case ")]
	for _, item := range []struct {
		version, build string
		valid          bool
	}{{"1.2.3", "42", true}, {"0.0.1", "1.2.3", true}, {"1.2", "1", false}, {"1.2.3", "1.2.3.4", false}, {"1.2.3;secret", "1", false}, {"1.2.3", "-1", false}} {
		t.Setenv("IOS_VERSION", item.version)
		t.Setenv("IOS_BUILD_NUMBER", item.build)
		_, err := toolOutput(context.Background(), toolCommand{Workspace: root, Executable: "bash", Args: []string{"-eu"}, Stdin: []byte(prefix), ExtraEnvNames: []string{"IOS_VERSION", "IOS_BUILD_NUMBER"}})
		if (err == nil) != item.valid {
			t.Fatal("实际模板版本边界错误")
		}
	}
	copy := IOSTemplate()
	copy[0] = '!'
	if IOSTemplate()[0] == '!' {
		t.Fatal("模板返回共享可变字节")
	}
	if runtime.GOOS != "darwin" {
		return
	}
	// 执行真实归档核验片段；匹配成功，参数与产物不匹配明确失败。
	app := filepath.Join(root, "App.xcarchive", "Products", "Applications", "App.app")
	if os.MkdirAll(app, 0700) != nil {
		t.Fatal("测试目录创建失败")
	}
	plist := `<?xml version="1.0"?><plist version="1.0"><dict><key>CFBundleIdentifier</key><string>org.mybuilds.fixture</string><key>CFBundleShortVersionString</key><string>1.2.3</string><key>CFBundleVersion</key><string>42</string></dict></plist>`
	if os.WriteFile(filepath.Join(app, "Info.plist"), []byte(plist), 0600) != nil {
		t.Fatal("测试plist写入失败")
	}
	verification := runs[0][strings.Index(runs[0], "apps=("):]
	for _, build := range []string{"42", "43"} {
		t.Setenv("MYBUILDS_IOS_BUILD_DIR", root)
		t.Setenv("IOS_VERSION", "1.2.3")
		t.Setenv("IOS_BUILD_NUMBER", build)
		t.Setenv("MYBUILDS_IOS_BUNDLE_ID", "org.mybuilds.fixture")
		_, err := toolOutput(context.Background(), toolCommand{Workspace: root, Executable: "bash", Args: []string{"-eu"}, Stdin: []byte(verification), ExtraEnvNames: []string{"MYBUILDS_IOS_BUILD_DIR", "IOS_VERSION", "IOS_BUILD_NUMBER", "MYBUILDS_IOS_BUNDLE_ID"}})
		if (err == nil) != (build == "42") {
			t.Fatal("实际归档版本核验未按产物拒绝错号")
		}
	}
}

// 模板通过真实配置与参数消费者验证；本测试不运行工具或读取签名文件。
func TestIOSTemplatePureConfiguration(t *testing.T) {
	document, err := config.Parse(IOSTemplate())
	if err != nil {
		t.Fatal(err)
	}
	names, err := document.Select([]string{"ios"}, false)
	if err != nil || len(names) != 1 || names[0] != "ios" {
		t.Fatalf("模板构建选择错误: %v", err)
	}
	build := document.Builds["ios"]
	if build.Runner == nil || build.Runner.Platform != "ios" || build.IOSSigning == nil {
		t.Fatal("模板缺少明确平台或签名定义")
	}
	params := map[string]string{"xcode_project": "App.xcodeproj", "scheme": "App", "bundle_id": "com.example.app", "version": "1.2.3", "build_number": "42"}
	for missing := range params {
		values := map[string]string{}
		for key, value := range params {
			if key != missing {
				values[key] = value
			}
		}
		if _, err := config.ResolveParams(build, values); err == nil {
			t.Fatal("模板缺失必填参数却通过")
		}
	}
	values, err := config.ResolveParams(build, params)
	if err != nil || values["export_method"] != "debugging" {
		t.Fatalf("模板默认导出方式错误: %v", err)
	}
	for _, step := range build.Steps {
		if step.Kind != "run" && step.Kind != "artifact" {
			t.Fatal("模板含未经交付的发布动作")
		}
	}
	changed := IOSTemplate()
	changed[0] = '!'
	if IOSTemplate()[0] == '!' {
		t.Fatal("调用者修改共享模板字节")
	}
}
