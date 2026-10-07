# 研究结论

- 现有服务端 token create 自动迁移，在线数据库管理受排他锁；首次初始化先离线创建，后启动服务；节点可用在线客户端登记。
- Windows YAML明文token触发0600校验不可用；用户环境变量提供token是现有受支持路径。Windows原生产物下载被实现拒绝，应明确边界，完整操作用WSL。
- 指定服务器是macOS arm64，可sudo；SSH会话无GUI不保证LaunchAgent用户域，使用显式系统模式LaunchDaemon并以目标用户运行。
- 不安装SDK、不增加Web服务，不开放防火墙。SSH转发复用现有加密传输。
- 选择可审查本地脚本与SHA256发行包；不引入包管理器仓库或自定义更新服务。
