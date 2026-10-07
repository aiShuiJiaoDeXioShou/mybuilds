---
name: mybuilds-deploy
description: 安装、初始化或维护 mybuilds 客户端、控制端与构建 Agent，配置客户端连接和系统服务。适用于首次部署、跨机器连接和安装排障。
---

# mybuilds 部署

先确认用户指定的目标机器、角色与已有安装。用 `mybuilds version`、配置文件是否存在和服务状态判断，不打印 token。部署授权只覆盖用户指定机器与服务；不要顺带修改防火墙、其他服务或 SDK。

## 选择入口

仓库：[aiShuiJiaoDeXioShou/mybuilds](https://github.com/aiShuiJiaoDeXioShou/mybuilds)。读取选定版本的 `docs/INSTALL.md`，下载同一版本安装脚本和包。安装器校验 SHA-256，禁止跳过校验。

- macOS/Linux：`bash install-client.sh`；服务端 `bash install-server.sh --with-agent`。
- SSH 无 GUI 的 macOS 服务器：以普通服务用户执行 `bash install-server.sh --with-agent --service system`，需该用户可用 sudo；生成以该用户运行的 LaunchDaemon。
- Windows 原生客户端：`./install-client.ps1`。Windows 服务端：`./install-server.ps1 -Distro Ubuntu -WithAgent`，要求已有 WSL2、systemd及非root用户。不要自动关闭或重启其他 WSL 发行版。
- Unix客户端连接：`bash install-client.sh --server-url https://build.example.com --token-file /private/path/token`。token 文件本人所有、0600；不把 token 作为命令参数或输出。已有配置保留。
- Windows原生配置不写token；安装器从私有TokenFile导入用户环境变量 `MYBUILDS_CLIENT_TOKEN`。环境变量优先于文件。原生 `artifact download` 尚不支持，完整操作使用 WSL。

## 角色与默认路径

`mybuilds-server` 只管理队列、权限和记录；`mybuilds-agent` 执行构建。没有内置或自动存在的默认节点；`--with-agent` 才登记并启动同机 `local`。项目 `--default-node local` 不会创建节点。

默认配置在 `~/.mybuilds/{client,server,agent}.yml`，Unix程序在 `~/.local/bin`。服务器管理员身份在 client.yml，节点使用 agent.yml 中独立token，不能混用。Windows程序在 `%LOCALAPPDATA%/mybuilds/bin`。

首次离线初始化可用 `mybuilds-server token create --role admin`，它自动迁移数据库。在线服务独占数据库，包括只读的 `mybuilds-server …` 管理命令也不能并行使用；在线管理改用 `mybuilds`。重复安装不能重新生成身份或覆盖配置；安装器拒绝不同版本，需要先按文档停机备份。

## 验证与排障

1. `mybuilds version` 验证 PATH；安装 shell 子进程不能修改父进程，用新终端或程序绝对路径。
2. `mybuilds status --json` 验证连接；`mybuilds node show local --json` 的 `session_active: true` 才表示节点在线。
3. 根据明确的测试请求注册可信演示仓库并执行构建；没有 runner 的流水线必须同时设置 `--nodes local --default-node local`。
4. 服务与Agent分别托管。macOS查看 `launchctl print gui/UID/io.mybuilds.server` 或系统模式 `sudo launchctl print system/io.mybuilds.server`；Linux查看 `systemctl --user status io.mybuilds.server`。日志目录 `~/.mybuilds/logs` 或 journalctl。
5. 缺少 SDK 不妨碍通用 shell 任务，但对应移动模板不能执行。`mybuilds-agent doctor` 可能因可选工具缺失返回非零，不能据此声称整个 Agent 无法工作。

控制端默认只监听 `127.0.0.1:8787`。跨机器使用验证证书的 HTTPS，或已授权 SSH 隧道映射到本地回环地址；不得以禁用证书验证或公网明文token请求修复连接。变更已有凭据、停机、删除数据或发布操作须属于当前用户授权范围。
