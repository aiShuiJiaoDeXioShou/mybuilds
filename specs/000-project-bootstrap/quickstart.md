# 初始化验收

在项目根目录操作，要求 Go 1.25 起与 Git；首次下载依赖需要访问 Go 模块源。

```bash
go mod download
go test ./...
go vet ./...
go run ./cmd/mybuilds --help
go run ./cmd/mybuilds version
go run ./cmd/mybuilds-server --help
go run ./cmd/mybuilds-server version
mkdir -p bin
go build -o bin/mybuilds ./cmd/mybuilds
go build -o bin/mybuilds-server ./cmd/mybuilds-server
./bin/mybuilds version
./bin/mybuilds-server version
```

两端版本均应为 `dev (commit: unknown, built: unknown)`；帮助显示各自入口及 version。

```bash
./bin/mybuilds does-not-exist
./bin/mybuilds-server does-not-exist
./bin/mybuilds version extra
./bin/mybuilds-server version extra
```

以上每个命令均须非零退出；不能输出成功版本。单独无参数运行两端应显示帮助并成功。

```bash
go build -ldflags '-X mybuilds/internal/version.Version=0.0.1 -X mybuilds/internal/version.Commit=demo -X mybuilds/internal/version.BuildDate=2026-10-04' -o bin/mybuilds ./cmd/mybuilds
./bin/mybuilds version
GOOS=linux GOARCH=amd64 go build -o bin/mybuilds-linux ./cmd/mybuilds
GOOS=windows GOARCH=amd64 go build -o bin/mybuilds-windows.exe ./cmd/mybuilds
```

注入后应输出 `0.0.1 (commit: demo, built: 2026-10-04)`；交叉编译只检查生成结果，不在 macOS 执行。
服务端按相同 ldflags 注入并核对；完成后按默认构建命令恢复两端 bin。
检查 README 所有本地文档链接、AGENTS 阅读指引与 PLAN 新目录；确认 001 未标记完成。
将实际验证结果记录在 [validation.md](validation.md)。
