package mobile

import _ "embed"

//go:embed templates/native-ios.yml
var iosTemplate []byte

// IOSTemplate 返回独立副本，初始化后的配置由用户在仓库内编辑。
func IOSTemplate() []byte { return append([]byte(nil), iosTemplate...) }
