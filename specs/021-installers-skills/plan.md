# 实施计划：跨平台安装与操作技能

## Summary
复用现有三 CLI，发行预编译包，无新增业务依赖。Shell 下载入口校验平台包后调用包内安装器；Windows PowerShell 安装原生客户端，服务端转交现有 WSL。

## Technical Context
Bash 3.2+、Python 3（Unix安装配置、服务文件及 JSON 处理）、PowerShell 5.1+；Go 1.25 构建发行包。macOS launchd，Linux systemd --user。安装目录默认 ~/.local/bin，配置 ~/.mybuilds，技能 ~/.codex/skills。可显式改安装目录供隔离验收。

## Constitution Check
沿用单模块和三进程角色；不改调度、数据库、发布保护或平台支持。不新增 Go 依赖。敏感配置私有且不覆盖；只监听回环。保留可运行安装检查、中文说明。既有原则2.1.0完整，无需重做constitution。

## Structure / 文件归属
主代理负责 scripts/install*.sh、scripts/install*.ps1、scripts/install.py、scripts/release.py、scripts/test-install.py、skills/mybuilds-{deploy,operate}/SKILL.md、README.md、docs/INSTALL.md、docs/CLI.md、specs/021-installers-skills/。研究代理只读核对 Windows 和服务约束，无共享文件并发写入。

## Decisions
- 用户级安装，无强制sudo。macOS使用用户域 LaunchAgent；SSH无GUI时可由普通用户通过sudo安装以该UserName运行的LaunchDaemon。Linux使用systemd用户服务，开机无登录需管理员启用linger。
- 发行包包含平台二进制、安装器与两个技能；macOS保持cgo供iOS能力，Linux/Windows不启用cgo。Windows原生客户端下载制品仍受现有实现限制，完整操作可用WSL；不伪造原生支持。
- Unix配置用Python标准库JSON序列化为合法YAML，无拼接注入；Windows配置不含token，通过用户环境变量提供，避免现有POSIX权限校验不兼容。
- 远程实际为macOS arm64；本次使用指定用户的LaunchDaemon，控制端和Agent独立服务；本地SSH密钥限端口转发，托管SSH隧道连接回环服务。
- 服务端安装为新安装引导，重跑保留配置；不会隐式升级在运行二进制或修改人工配置。遇到不同已有安装拒绝覆盖，版本相同则幂等。
- 初次公开推送先检查已跟踪内容不含运行凭据；服务器密码及实际地址只留部署环境，不写入Git。

## Verification
隔离目录真实安装、幂等与失败保护，真实临时控制端与Agent；Shell语法、PowerShell解析/WindowsCI，跨平台构建；实际远程服务与本地完整构建。公开发布后再次用下载入口安装，记录验证边界。
