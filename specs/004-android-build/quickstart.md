# 004 真实验收指南

所有工程构建在examples/android的临时副本，不写用户工程/SDK，不读取未知密钥。

1. 编译客户端到临时二进制目录；记录OS/Go/java/SDK，显式JAVA_HOME/ANDROID_HOME指向已有工具。
2. doctor --platform android --json --working-dir临时工程，核对实际Java/SDK/wrapper，未声明签名为skipped。
3. keytool在临时目录生成JKS，alias mybuilds_test，store/key密码均用:env；设置ANDROID_KEYSTORE/ANDROID_KEY_ALIAS/ANDROID_KEYSTORE_PASSWORD/ANDROID_KEY_PASSWORD。显式doctor签名flags验证。
4. 复制工程中init --framework native --platform android；先dry-run，再run --param version=1.2.3 --param build_number=42。保存脱敏JSON/stderr/result_dir。
5. aapt核对APK identity/versionCode42/versionName1.2.3，apksigner verify核对证书；AAB复用AGP运行时官方bundletool1.18.1核对真实manifest，jarsigner核对签名；mapping非空并含示例类；所有快照大小/SHA256重算一致。
6. 再次wrapper --offline构建相同任务，记录缓存复用且SDK未更新。
7. 缺/错Java/SDK/wrapper、store/key密码、alias/非私钥各失败；秘密不回显。取消真实构建，确认本次进程退出且无关daemon存活。
8. go test ./...、go test -race ./internal/mobile ./internal/pipeline ./internal/process、go vet ./...；Linux/Windows交叉编译不能代替真实宿主构建。

环境或网络不足时validation.md列已通过项、待验证命令和证据路径，不宣称完整验收。

## 无额外下载的 AAB manifest 核验

在临时工程中，签名环境沿用示例README；`AAB_SNAPSHOT`设置为成功结果JSON中result_dir与AAB snapshot_path拼接后的绝对路径。只读独立快照，不读取失败构建覆盖后的工作区文件。

```bash
export APP_VERSION=1.2.3 BUILD_NUMBER=42
export AAB_SNAPSHOT="/成功结果的result_dir/artifacts/android/android-artifacts/files/app/build/outputs/bundle/release/app-release.aab"
verification_script="$(mktemp)"
cat > "$verification_script" <<'GRADLE'
gradle.projectsEvaluated {
    def app = gradle.rootProject.project(':app')
    app.tasks.register('verifyBundleManifest') {
        doLast {
            def loader = app.plugins.getPlugin('com.android.application').class.classLoader
            def parser = loader.loadClass('com.android.tools.build.bundletool.flags.FlagParser').getConstructor().newInstance()
            def flags = parser.parse(['dump', 'manifest', '--bundle=' + System.getenv('AAB_SNAPSHOT')] as String[])
            loader.loadClass('com.android.tools.build.bundletool.commands.DumpCommand').fromFlags(flags).execute()
        }
    }
}
GRADLE
./gradlew :app:verifyBundleManifest --init-script "$verification_script" --no-daemon --offline -Pandroid.builder.sdkDownload=false
```

实际使用模块版本/Google Maven来源与摘要见research.md；输出manifest应包含package=com.example.mybuilds、android:versionCode=42、android:versionName=1.2.3。独立bundletool CLI仅可选，不是本期必须下载的工具。
