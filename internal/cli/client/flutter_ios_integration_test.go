package client

import (
	"os"
	"strings"
	"testing"

	"mybuilds/internal/config"
)

func TestFlutterIOSAndDualTypedInitPreview(t *testing.T) {
	for _, platforms := range []string{"ios", "android,ios"} {
		t.Run(platforms, func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("IOS_P12_PASSWORD", "UNREAD_JOINT_SIGNING_SECRET")
			if _, err := execute(t, "init", "--framework", "flutter", "--platform", platforms); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile("mybuilds.yml")
			if err != nil {
				t.Fatal(err)
			}
			doc, err := config.Parse(data)
			if err != nil || doc.Builds["ios"] == nil || doc.Builds["ios"].IOSSigning == nil || doc.Builds["ios"].Runner.Framework != "flutter" {
				t.Fatal("真实五字段IOSSigning未接入Flutter", err)
			}
			args := []string{"run", "--dry-run", "--all", "--param", "ios:xcode_project=ios/Runner.xcworkspace", "--param", "ios:scheme=Runner", "--param", "ios:bundle_id=com.example.flutter"}
			if strings.Contains(platforms, "android") {
				args = append(args, "--param", "android:application_id=com.example.flutter")
			}
			out, err := execute(t, args...)
			if err != nil || strings.Contains(out, "UNREAD_JOINT_SIGNING_SECRET") {
				t.Fatal("双平台纯预览", err)
			}
			if _, err := execute(t, "run", "--dry-run"); platforms != "ios" && err == nil {
				t.Fatal("多build未选仍执行")
			}
		})
	}
}
