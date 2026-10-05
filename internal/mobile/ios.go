package mobile

import (
	"regexp"
)

// IOSSigningOptions 只在内部资源生命周期传递；密码不进入公共结果。
type IOSSigningOptions struct {
	Workspace, OutputDir                                   string
	P12File, ProfileFile, Password, BundleID, ExportMethod string
}

type IOSDoctorOptions struct {
	Workspace string
	Signing   *IOSSigningOptions
}

var iosBundleID = regexp.MustCompile(`^[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+$`)
var iosProfileUUID = regexp.MustCompile(`^[A-Fa-f0-9]{8}-[A-Fa-f0-9]{4}-[A-Fa-f0-9]{4}-[A-Fa-f0-9]{4}-[A-Fa-f0-9]{12}$`)
var iosTeamID = regexp.MustCompile(`^[A-Z0-9]{10}$`)

func validIOSMethod(method string) bool {
	switch method {
	case "debugging", "release-testing", "app-store-connect", "enterprise":
		return true
	}
	return false
}

// IOSSigningSupported 只报告已编译的原生组件与宿主API，不代表任何证书材料有效。
func IOSSigningSupported() bool { return iosNativeAvailable() }
