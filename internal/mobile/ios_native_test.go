//go:build darwin && cgo

package mobile

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type iosPrototypeInput struct{ P12, Keychain, Binary, Identity, Password string }

func TestIOSNativePrototypeChild(t *testing.T) {
	if os.Getenv("MYBUILDS_IOS_NATIVE_CHILD") != "1" {
		t.Skip("仅作为真实原型的受限子进程运行")
	}
	var input iosPrototypeInput
	if json.NewDecoder(os.Stdin).Decode(&input) != nil {
		t.Fatal("原型输入非法")
	}
	p12, err := os.ReadFile(input.P12)
	if err != nil {
		t.Fatal("原型材料无法读取")
	}
	// 真实错误密码会半创建keychain，仍须原生删除。
	if iosNativeImport(p12, "wrong-password", input.Keychain, "generated-keychain-password") == nil {
		t.Fatal("错误P12密码未拒绝")
	}
	if iosNativeDelete(input.Keychain) != nil {
		t.Fatal("半准备keychain未清理")
	}
	if iosNativeImport(p12, input.Password, input.Keychain, "generated-keychain-password") != nil {
		t.Fatal("真实原型导入失败")
	}
	defer func() {
		if iosNativeDelete(input.Keychain) != nil {
			t.Error("真实原型keychain清理失败")
		}
	}()
	command := exec.Command("/usr/bin/security", "-i", "-q")
	command.Stdin = strings.NewReader("set-key-partition-list -S apple: -s -k generated-keychain-password \"" + input.Keychain + "\"\n")
	command.Stdout = &bytes.Buffer{}
	command.Stderr = &bytes.Buffer{}
	if command.Run() != nil {
		t.Fatal("真实partition匿名stdin失败")
	}
	command = exec.Command("/usr/bin/codesign", "--force", "--sign", input.Identity, "--keychain", input.Keychain, input.Binary)
	command.Stdout = &bytes.Buffer{}
	command.Stderr = &bytes.Buffer{}
	if command.Run() != nil {
		t.Fatal("明确临时私钥非交互签名失败")
	}
	command = exec.Command("/usr/bin/codesign", "--verify", "--strict", input.Binary)
	command.Stdout = &bytes.Buffer{}
	command.Stderr = &bytes.Buffer{}
	if command.Run() != nil {
		t.Fatal("自产MachO签名核验失败")
	}
	// 用户run替换叶文件时，资源所有者必须在调用SecKeychainDelete之前拒绝。
	directory := filepath.Dir(input.Keychain)
	temporaryInfo, _ := os.Lstat(directory)
	ownedFiles := map[string]os.FileInfo{}
	leaf := ""
	for _, filename := range []string{input.Keychain, input.Keychain + "-db"} {
		if info, e := os.Lstat(filename); e == nil {
			ownedFiles[filename] = info
			leaf = filename
		}
	}
	other := filepath.Join(directory, "other.keychain")
	if iosNativeImport(p12, input.Password, other, "other-password") != nil {
		t.Fatal("第二枚自有keychain创建失败")
	}
	defer iosNativeDelete(other)
	otherLeaf := other
	if _, e := os.Stat(otherLeaf); os.IsNotExist(e) {
		otherLeaf += "-db"
	}
	replacement, e := os.ReadFile(otherLeaf)
	if e != nil {
		t.Fatal("第二枚自有keychain不可读")
	}
	if os.Rename(leaf, leaf+".saved") != nil {
		t.Fatal("自有原叶文件保存失败")
	}
	if os.WriteFile(leaf, replacement, 0600) != nil {
		t.Fatal("替换测试叶文件失败")
	}
	replacementInfo, _ := os.Lstat(leaf)
	resources := &IOSResources{workspace: directory, temporary: directory, temporaryInfo: temporaryInfo, keychain: input.Keychain, keychainInfo: ownedFiles, env: map[string]string{}}
	if resources.Close(context.Background()) != ErrIOSCleanup {
		t.Fatal("keychain替换未报告清理失败")
	}
	after, e := os.Lstat(leaf)
	if e != nil || !os.SameFile(after, replacementInfo) {
		t.Fatal("未知替换keychain被删除")
	}
	if os.Remove(leaf) != nil || os.Rename(leaf+".saved", leaf) != nil {
		t.Fatal("自产原叶文件恢复失败")
	}

}

func TestIOSNativeKeychainPrototype(t *testing.T) {
	if !iosNativeAvailable() {
		t.Skip("macOS15+cgo是内存导入的必要条件")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	snapshot := func() string {
		var output string
		for _, args := range [][]string{{"default-keychain", "-d", "user"}, {"list-keychains", "-d", "user"}} {
			value, e := toolOutput(ctx, toolCommand{Workspace: root, Executable: "/usr/bin/security", Args: args})
			if e != nil {
				t.Fatal("无法取得只读keychain列表摘要")
			}
			output += value
		}
		return output
	}
	before := snapshot()
	run := func(executable string, args []string, stdin string) string {
		out, e := toolOutput(ctx, toolCommand{Workspace: root, Executable: executable, Args: args, Stdin: []byte(stdin)})
		if e != nil {
			t.Fatal("自产原型工具失败：", executable)
		}
		return out
	}
	key := root + "/test.key"
	cert := root + "/test.crt"
	p12 := root + "/test.p12"
	binary := root + "/tiny"
	run("/usr/bin/openssl", []string{"req", "-x509", "-newkey", "rsa:2048", "-nodes", "-keyout", key, "-out", cert, "-subj", "/CN=mybuilds owned signing prototype", "-days", "1", "-addext", "keyUsage=digitalSignature", "-addext", "extendedKeyUsage=codeSigning"}, "")
	password := "p12-sensitive-${DO_NOT_READ}"
	run("/usr/bin/openssl", []string{"pkcs12", "-export", "-inkey", key, "-in", cert, "-out", p12, "-passout", "stdin"}, password+"\n")
	der := run("/usr/bin/openssl", []string{"x509", "-in", cert, "-outform", "DER"}, "")
	fingerprint := sha1.Sum([]byte(der))
	identity := strings.ToUpper(hex.EncodeToString(fingerprint[:]))
	if os.WriteFile(root+"/main.c", []byte("int main(void){return 0;}\n"), 0600) != nil {
		t.Fatal("写自产源码失败")
	}
	run("/usr/bin/clang", []string{root + "/main.c", "-o", binary}, "")
	input, _ := json.Marshal(iosPrototypeInput{P12: p12, Keychain: root + "/signing.keychain", Binary: binary, Identity: identity, Password: password})
	executable, _ := os.Executable()
	t.Setenv("MYBUILDS_IOS_NATIVE_CHILD", "1")
	output, e := toolOutput(ctx, toolCommand{Workspace: root, Executable: executable, Args: []string{"-test.run=^TestIOSNativePrototypeChild$"}, ExtraEnvNames: []string{"MYBUILDS_IOS_NATIVE_CHILD"}, Stdin: input})
	if e != nil {
		t.Fatalf("真实签名子进程失败（固定诊断）：%s", strings.TrimSpace(output))
	}
	if strings.Contains(output, password) {
		t.Fatal("子进程泄漏密码")
	}
	if before != snapshot() {
		t.Fatal("用户default/search list改变")
	}
	for _, filename := range []string{root + "/signing.keychain", root + "/signing.keychain-db"} {
		if _, e = os.Stat(filename); !os.IsNotExist(e) {
			t.Fatal("真实keychain残留")
		}
	}
	// 自签CMS不能冒充Apple profile；此处只验证拒绝边界，不声称真实Apple IPA通过。
	profile := root + "/fake.mobileprovision"
	plist := root + "/fake.plist"
	if os.WriteFile(plist, []byte(`<?xml version="1.0"?><plist version="1.0"><dict><key>UUID</key><string>00000000-0000-0000-0000-000000000000</string></dict></plist>`), 0600) != nil {
		t.Fatal("写自产profile失败")
	}
	run("/usr/bin/openssl", []string{"cms", "-sign", "-binary", "-nodetach", "-in", plist, "-signer", cert, "-inkey", key, "-outform", "DER", "-out", profile}, "")
	err = ValidateIOSSigning(ctx, IOSSigningOptions{Workspace: root, P12File: p12, ProfileFile: profile, Password: password, BundleID: "org.mybuilds.prototype", ExportMethod: "debugging"})
	cwd, e := os.Getwd()
	if e != nil {
		t.Fatal("当前目录不可用")
	}
	relative, e := filepath.Rel(cwd, root)
	if e != nil {
		t.Fatal("相对测试目录不可用")
	}
	relativeErr := ValidateIOSSigning(ctx, IOSSigningOptions{Workspace: relative, P12File: "test.p12", ProfileFile: "fake.mobileprovision", Password: password, BundleID: "org.mybuilds.prototype", ExportMethod: "debugging"})
	if relativeErr == nil || relativeErr.Error() != "ios_profile_untrusted" {
		t.Fatal("相对workspace与材料被重复拼接")
	}
	if err == nil || err.Error() != "ios_profile_untrusted" {
		t.Fatal("自签CMS未在固定Apple锚处拒绝")
	}
	if before != snapshot() {
		t.Fatal("只读预检查改变用户keychain")
	}
	// 外部TMPDIR与仓库内TMPDIR两种情况均执行真实Prepare半失败清理。
	external, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal("自有外部临时根不可用")
	}
	inside := root + "/temporary"
	if os.Mkdir(inside, 0700) != nil {
		t.Fatal("自有内部临时根创建失败")
	}
	for _, temporaryRoot := range []string{external, inside} {
		t.Setenv("TMPDIR", temporaryRoot)
		_, err = PrepareIOSResources(ctx, IOSSigningOptions{Workspace: root, OutputDir: "owned-output", P12File: p12, ProfileFile: profile, Password: password, BundleID: "org.mybuilds.prototype", ExportMethod: "debugging"})
		if err == nil || err.Error() != "ios_profile_untrusted" {
			t.Fatal("半准备未按真实profile拒绝")
		}
		if _, e := os.Stat(root + "/owned-output"); !os.IsNotExist(e) {
			t.Fatal("半准备输出目录残留")
		}
		if entries, e := os.ReadDir(temporaryRoot); e != nil || len(entries) != 0 {
			t.Fatal("半准备临时目录残留或私有资源进入仓库")
		}
	}
}
