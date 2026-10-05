package mobile

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"mybuilds/internal/process"
)

func TestIOSUnsignedArchive(t *testing.T) {
	if runtime.GOOS != "darwin" || os.Getenv("MYBUILDS_IOS_UNSIGNED_TEST") != "1" {
		t.Skip("仅在明确启用的Xcode宿主做实际无签名归档")
	}
	root := t.TempDir()
	project := filepath.Join(root, "App.xcodeproj")
	if os.Mkdir(project, 0700) != nil {
		t.Fatal("自产工程目录创建失败")
	}
	if os.WriteFile(filepath.Join(project, "project.pbxproj"), []byte(iosUnsignedProject), 0600) != nil {
		t.Fatal("自产工程写入失败")
	}
	source := `#import <UIKit/UIKit.h>
int main(int argc,char **argv) { @autoreleasepool { return UIApplicationMain(argc,argv,nil,nil); } }
`
	if os.WriteFile(filepath.Join(root, "main.m"), []byte(source), 0600) != nil {
		t.Fatal("自产源码写入失败")
	}
	env := []string{}
	for key, value := range process.HostEnvironment() {
		env = append(env, key+"="+value)
	}
	slices.Sort(env)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	result := process.Run(ctx, process.Command{Path: "/usr/bin/xcodebuild", Dir: root, Env: env, Args: []string{"-quiet", "-project", project, "-scheme", "App", "-configuration", "Release", "-sdk", "iphoneos", "-destination", "generic/platform=iOS", "-derivedDataPath", root + "/DerivedData", "-archivePath", root + "/App.xcarchive", "archive", "CODE_SIGNING_ALLOWED=NO"}}, io.Discard, io.Discard)
	if result.ExitCode != 0 || result.Reason != "" || result.CleanupFailed {
		t.Fatalf("无签名归档失败：%s %d", result.Reason, result.ExitCode)
	}
	apps, err := filepath.Glob(root + "/App.xcarchive/Products/Applications/*.app")
	if err != nil || len(apps) != 1 {
		t.Fatal("无签名归档app缺失")
	}
	symbols, err := filepath.Glob(root + "/App.xcarchive/dSYMs/*.dSYM")
	if err != nil || len(symbols) != 1 {
		t.Fatal("无签名归档dSYM缺失")
	}
	runs := iosTemplateRuns(t)
	verification := runs[0][strings.Index(runs[0], "apps=("):]
	t.Setenv("MYBUILDS_IOS_BUILD_DIR", root)
	t.Setenv("IOS_VERSION", "1.2.3")
	t.Setenv("IOS_BUILD_NUMBER", "42")
	t.Setenv("MYBUILDS_IOS_BUNDLE_ID", "org.mybuilds.fixture")
	if _, err = toolOutput(ctx, toolCommand{Workspace: root, Executable: "bash", Args: []string{"-eu"}, Stdin: []byte(verification), ExtraEnvNames: []string{"MYBUILDS_IOS_BUILD_DIR", "IOS_VERSION", "IOS_BUILD_NUMBER", "MYBUILDS_IOS_BUNDLE_ID"}}); err != nil {
		t.Fatal("实际无签名归档版本核验失败")
	}
	if _, err = toolOutput(ctx, toolCommand{Workspace: root, Executable: "/usr/bin/ditto", Args: []string{"-c", "-k", "--keepParent", root + "/App.xcarchive/dSYMs", root + "/App.dSYM.zip"}}); err != nil {
		t.Fatal("真实dSYM压缩失败")
	}
	if info, err := os.Stat(root + "/App.dSYM.zip"); err != nil || info.Size() == 0 {
		t.Fatal("真实dSYM压缩产物为空")
	}
}

const iosUnsignedProject = `// !$*UTF8*$!
{archiveVersion = 1; classes = {}; objectVersion = 56;
objects = {
A00000000000000000000001 = {isa = PBXProject; buildConfigurationList = A00000000000000000000002; compatibilityVersion = "Xcode 14.0"; mainGroup = A00000000000000000000003; productRefGroup = A00000000000000000000004; projectDirPath = ""; projectRoot = ""; targets = (A00000000000000000000005); };
A00000000000000000000002 = {isa = XCConfigurationList; buildConfigurations = (A00000000000000000000006,A00000000000000000000007); defaultConfigurationIsVisible = 0; defaultConfigurationName = Release; };
A00000000000000000000003 = {isa = PBXGroup; children = (A00000000000000000000008,A00000000000000000000004); sourceTree = "<group>"; };
A00000000000000000000004 = {isa = PBXGroup; children = (A00000000000000000000009); name = Products; sourceTree = "<group>"; };
A00000000000000000000005 = {isa = PBXNativeTarget; buildConfigurationList = A00000000000000000000010; buildPhases = (A00000000000000000000011); buildRules = (); dependencies = (); name = App; productName = App; productReference = A00000000000000000000009; productType = "com.apple.product-type.application"; };
A00000000000000000000006 = {isa = XCBuildConfiguration; buildSettings = {CLANG_ENABLE_MODULES = YES; CLANG_ENABLE_OBJC_ARC = YES; SDKROOT = iphoneos; IPHONEOS_DEPLOYMENT_TARGET = 17.0;}; name = Debug;};
A00000000000000000000007 = {isa = XCBuildConfiguration; buildSettings = {CLANG_ENABLE_MODULES = YES; CLANG_ENABLE_OBJC_ARC = YES; SDKROOT = iphoneos; IPHONEOS_DEPLOYMENT_TARGET = 17.0;}; name = Release;};
A00000000000000000000008 = {isa = PBXFileReference; lastKnownFileType = sourcecode.c.objc; path = main.m; sourceTree = "<group>"; };
A00000000000000000000009 = {isa = PBXFileReference; explicitFileType = wrapper.application; includeInIndex = 0; path = App.app; sourceTree = BUILT_PRODUCTS_DIR; };
A00000000000000000000010 = {isa = XCConfigurationList; buildConfigurations = (A00000000000000000000012,A00000000000000000000013); defaultConfigurationIsVisible = 0; defaultConfigurationName = Release; };
A00000000000000000000011 = {isa = PBXSourcesBuildPhase; buildActionMask = 2147483647; files = (A00000000000000000000014); runOnlyForDeploymentPostprocessing = 0; };
A00000000000000000000012 = {isa = XCBuildConfiguration; buildSettings = {PRODUCT_NAME = "$(TARGET_NAME)"; PRODUCT_BUNDLE_IDENTIFIER = org.mybuilds.fixture; GENERATE_INFOPLIST_FILE = YES; CURRENT_PROJECT_VERSION = 42; MARKETING_VERSION = 1.2.3; TARGETED_DEVICE_FAMILY = "1,2"; DEBUG_INFORMATION_FORMAT = "dwarf-with-dsym"; OTHER_LDFLAGS = ("-framework",UIKit); }; name = Debug; };
A00000000000000000000013 = {isa = XCBuildConfiguration; buildSettings = {PRODUCT_NAME = "$(TARGET_NAME)"; PRODUCT_BUNDLE_IDENTIFIER = org.mybuilds.fixture; GENERATE_INFOPLIST_FILE = YES; CURRENT_PROJECT_VERSION = 42; MARKETING_VERSION = 1.2.3; TARGETED_DEVICE_FAMILY = "1,2"; DEBUG_INFORMATION_FORMAT = "dwarf-with-dsym"; OTHER_LDFLAGS = ("-framework",UIKit); }; name = Release; };
A00000000000000000000014 = {isa = PBXBuildFile; fileRef = A00000000000000000000008; };
}; rootObject = A00000000000000000000001; }
`
