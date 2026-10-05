package client

import (
	"strings"
	"testing"
)

func TestPublishDoctorRejectsAllBuildSpecificFlags(t *testing.T) {
	for _, flag := range []string{"framework", "p12", "profile", "password-env", "bundle-id", "export-method"} {
		t.Run(flag, func(t *testing.T) {
			_, err := executeRemote(t, "doctor", "--target", "app-store", "--"+flag, "ignored")
			if err == nil || !strings.Contains(err.Error(), "不能混用") {
				t.Fatal("未先拒绝混用构建选项", err)
			}
		})
	}
}
