# 安装与服务管理

发行版本从 [GitHub Releases](https://github.com/aiShuiJiaoDeXioShou/mybuilds/releases) 下载。安装脚本默认 `v0.1.0`，下载对应架构的预编译包并核对 SHA-256；目标机不需要 Go。macOS/Linux 需要 Bash、curl、Python 3；服务端还需要 Git，移动 SDK 另行准备。

## macOS / Linux

下载客户端入口并执行：

```bash
curl -fsSL https://raw.githubusercontent.com/aiShuiJiaoDeXioShou/mybuilds/v0.1.0/scripts/install-client.sh -o install-client.sh
bash install-client.sh
```

服务端加同机 Agent：

```bash
curl -fsSL https://raw.githubusercontent.com/aiShuiJiaoDeXioShou/mybuilds/v0.1.0/scripts/install-server.sh -o install-server.sh
bash install-server.sh --with-agent
```

服务端默认安装用户服务。macOS 需要 GUI 登录会话；Linux 需要运行中的 systemd 用户管理器。SSH 专用服务器或需要开机后无人登录运行时，以**普通服务用户**执行：

```bash
bash install-server.sh --with-agent --service system
```

`system` 模式调用 `sudo -n`，要求已有可用 sudo 授权；程序仍以当前普通用户运行，不能直接以 root 安装。macOS 生成 LaunchDaemon，Linux 生成 systemd 系统服务。Linux 用户服务要在无登录时保持运行，可由管理员执行 `sudo loginctl enable-linger USER`。WSL 服务随该发行版运行，不代表 Windows 启动后自动启动 WSL。

首次服务端安装创建默认配置、管理员身份，以及可选的 `local` 节点独立身份。控制端只监听 `127.0.0.1:8787`，不修改防火墙。默认服务端配置不安装任何移动 SDK，不添加移动节点标签。

| 内容 | 默认位置 |
|---|---|
| 三个 Unix 程序 | `~/.local/bin` |
| 客户端/服务端/Agent 配置 | `~/.mybuilds/client.yml`、`server.yml`、`agent.yml` |
| 控制端数据/Agent 工作区 | `~/.mybuilds/server`、`~/.mybuilds/agent` |
| AI skills | `~/.codex/skills/mybuilds-deploy`、`mybuilds-operate` |
| macOS 服务日志 | `~/.mybuilds/logs` |

安装器把程序目录加入 `.profile`、`.bashrc`、`.zshrc`；新终端生效。当前终端可以执行 `export PATH="$HOME/.local/bin:$PATH"`。使用其他shell时自行设置PATH。有符号链接的启动文件不会被自动改写。

`--version vX.Y.Z` 必须作为下载脚本第一个选项；所有版本均从同名 release 下载。`--bin-dir`、`--config-dir`、`--skills-dir` 可改目录；`--no-path` 不修改shell配置。服务端支持 `--port`、`--with-agent` 和 `--service user|system|none`。`none` 完成初始化但不启动服务。自定义配置目录下，CLI使用 `--config /path/client.yml`。

`--with-agent` 只在首次安装时自动初始化节点；已有仅控制端的安装需按README手工登记与配置Agent。

已有配置始终保留。相同版本可重复安装，不重置身份、项目或数据；检测到不同二进制、不同技能文件或未由安装器创建的服务端配置时拒绝覆盖。不要将本次安装当作已有服务的自动迁移工具。

## 连接客户端

管理员将用户 token 通过私有渠道交给客户端，保存在本人所有的 `0600` 普通文件中。首次导入：

```bash
chmod 0600 /path/to/token
bash install-client.sh --server-url https://build.example.com --token-file /path/to/token
mybuilds status --json
```

私有 CA 可加 `--ca-file /path/to/ca.pem`。已有 `client.yml` 时保留原连接，安装器不替换其中的身份。环境变量 `MYBUILDS_CLIENT_TOKEN` 等仍会覆盖文件配置。

控制端默认没有 TLS 入口。跨机器可部署验证证书的 HTTPS 反向代理，或者使用已有 SSH 访问：

```bash
ssh -N -L 127.0.0.1:8787:127.0.0.1:8787 -p SSH_PORT USER@HOST
```

保持隧道运行，客户端 `server` 配置为 `http://127.0.0.1:8787`；若本地端口被占用，改转发左侧端口并同步配置。控制端与同机 Agent 始终通过远端回环通信。不要把远程明文 HTTP 作为跨机器连接地址。

## Windows

PowerShell 原生客户端（Windows PowerShell 5.1 或 PowerShell 7）：

```powershell
Invoke-WebRequest https://raw.githubusercontent.com/aiShuiJiaoDeXioShou/mybuilds/v0.1.0/scripts/install-client.ps1 -OutFile install-client.ps1
.\install-client.ps1
```

如果系统策略阻止本次脚本，可按本机管理员规定处理；安装器不修改全局执行策略。默认程序目录 `%LOCALAPPDATA%\mybuilds\bin`，安装器追加用户 PATH。可以传 `-Version`、`-BinDir`、`-ConfigDir`、`-SkillsDir`。

连接导入使用 `-ServerUrl https://build.example.com -TokenFile C:\private\token`。token文件ACL只允许本人、SYSTEM和Administrators读取。原生Windows的YAML凭据受现有0600校验限制，因此安装器把token保存到**用户环境变量** `MYBUILDS_CLIENT_TOKEN`，配置文件仅存服务地址；新终端读取。已有配置不覆盖，若曾自行建好配置但未设token，需另行设置用户环境变量。

原生客户端支持远程管理、配置生成与预览；**`artifact download` 当前不支持 Windows 原生环境**。需要完整下载、本地执行或服务端时使用 WSL：

```powershell
Invoke-WebRequest https://raw.githubusercontent.com/aiShuiJiaoDeXioShou/mybuilds/v0.1.0/scripts/install-server.ps1 -OutFile install-server.ps1
.\install-server.ps1 -Distro Ubuntu -WithAgent
wsl -d Ubuntu -- bash -lc 'mybuilds status --json'
```

需提前安装 WSL2 发行版、设置普通默认用户、安装 Python3/curl/Git，并按 [Microsoft说明](https://learn.microsoft.com/windows/wsl/systemd)启用systemd与用户服务。安装器不自动安装发行版、重启WSL或修改其他发行版。程序、配置和数据保存在Linux用户目录，不放 `/mnt/c`。也可在WSL中运行Unix客户端安装命令。

## 服务检查、停止与重启

```bash
mybuilds status --json
mybuilds node show local --json
```

Agent的 `session_active` 应为 `true`；通用任务无需移动SDK。`--default-node local` 仅指定项目节点，不会启动Agent。

macOS 用户服务：

```bash
launchctl print "gui/$(id -u)/io.mybuilds.server"
launchctl bootout "gui/$(id -u)/io.mybuilds.agent"
launchctl bootout "gui/$(id -u)/io.mybuilds.server"
launchctl bootstrap "gui/$(id -u)" "$HOME/Library/LaunchAgents/io.mybuilds.server.plist"
launchctl bootstrap "gui/$(id -u)" "$HOME/Library/LaunchAgents/io.mybuilds.agent.plist"
```

系统模式把域换为 `system`、路径换为 `/Library/LaunchDaemons/…`，命令加 `sudo`。Agent与控制端是两个服务，需要分别操作。

Linux 用户服务：

```bash
systemctl --user status io.mybuilds.server io.mybuilds.agent
journalctl --user -u io.mybuilds.server -u io.mybuilds.agent
systemctl --user stop io.mybuilds.agent io.mybuilds.server
systemctl --user start io.mybuilds.server io.mybuilds.agent
```

系统模式使用 `sudo systemctl` / `sudo journalctl`，去掉 `--user`。macOS日志见配置目录logs。

先 `mybuilds node drain local` 停接新任务，等待当前构建完成，再停止服务。直接停Agent会触发执行权撤销和恢复保护，不能视作无影响操作。需离线管理token或迁移数据库时先停控制端。

## 升级、移除与离线安装

本版安装器不自动升级运行中的程序。升级前停接单、等待任务结束、停止服务，备份整个配置和数据目录；将旧程序与两个技能目录移到备份位置，核对新版本迁移说明后用发行包中的二进制替换。安装器版本记录也需按新版本说明更新，不能通过删数据库“修复”升级。

移除先停服务并删除对应服务定义，再移除程序与shell PATH行。配置和数据默认保留，只有用户明确要求删除时再处理；技能可单独移除。Windows用户环境变量中的token需在取消连接时单独清除。

无法联网的目标机可通过可信渠道传入对应发行包和 `SHA256SUMS`，验证后解压，在包目录执行：

```bash
bash scripts/install.sh server --with-agent --service system
# 或 bash scripts/install.sh client
```

源码发布者使用 `python3 scripts/release.py --version vX.Y.Z --output dist`。可重复传 `--target darwin/arm64` 等；macOS包需要macOS主机构建以保留cgo/iOS能力。安装行为验收入口为 `scripts/test-install.py` 与 `scripts/test-install.ps1`。
