package config

import (
	"encoding/json"
	"strings"
	"testing"
)

// 真Parse检查框架与平台组合；原无Runner和native配置仍沿原语义。
func TestFlutterRunnerStrictInputAndFrozenDefinition(t *testing.T) {
	for _, runner := range []string{"", "runner: {platform: android}\n", "runner: {framework: native, platform: ios}\n", "runner: {framework: flutter, platform: android}\n", "runner: {framework: flutter, platform: ios}\n"} {
		doc, err := Parse([]byte("version: 1\n" + runner + "steps: [{kind: run, run: 'exit 0'}]\n"))
		if err != nil {
			t.Fatalf("合法框架组合拒绝：%v", err)
		}
		data, err := json.Marshal(doc.Builds["default"])
		if err != nil {
			t.Fatal(err)
		}
		var round Build
		if err = json.Unmarshal(data, &round); err != nil {
			t.Fatal(err)
		}
		if round.Runner != nil && round.Runner.Framework != doc.Builds["default"].Runner.Framework {
			t.Fatal("冻结定义丢失框架")
		}
	}
	for _, runner := range []string{"{framework: UNKNOWN_PRIVATE, platform: android}", "{framework: flutter}", "{framework: native}", "{framework: null, platform: android}", "{framework: '', platform: android}", "{framework: true, platform: android}", "{framework: [], platform: android}", "{framework: 1, platform: android}", "{framework: flutter, framework: native, platform: android}", "{framework: flutter, platform: unknown}"} {
		_, err := Parse([]byte("version: 1\nrunner: " + runner + "\nsteps: [{kind: run, run: 'exit 0'}]\n"))
		if err == nil || strings.Contains(err.Error(), "UNKNOWN_PRIVATE") {
			t.Fatal("非法框架被接纳或公开原值")
		}
	}
	_, err := Parse([]byte("version: 1\nparams: {mode: &mode flutter}\nrunner: {framework: *mode, platform: android}\nsteps: [{kind: run, run: 'exit 0'}]\n"))
	if err == nil {
		t.Fatal("框架alias未拒")
	}
	_, err = Parse([]byte("version: 1\nrunner: {<<: {framework: flutter}, platform: android}\nsteps: [{kind: run, run: 'exit 0'}]\n"))
	if err == nil {
		t.Fatal("框架merge未拒")
	}
	old, err := json.Marshal(Runner{Platform: "android"})
	if err != nil || string(old) != `{"Platform":"android","Labels":null}` {
		t.Fatal("未声明新字段改写旧定义字节")
	}
}
