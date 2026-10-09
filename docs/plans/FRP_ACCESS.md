# FRP 接入路线

2026-10-09：已完成 Spec Kit 规划，**尚未部署公网入口**。详细[实施计划](../../specs/023-frp-access/plan.md)、[配置示例](../../specs/023-frp-access/contracts/deployment.md)、[11项实施任务](../../specs/023-frp-access/tasks.md)、[验收与回退](../../specs/023-frp-access/quickstart.md)。

推荐链路：远程客户端/Agent → HTTPS域名:443 → 公网Caddy → FRP TCP转发 → 控制端主机`127.0.0.1:8787`。FRP作为独立服务运行，mybuilds无需内置隧道；同机Agent继续使用回环地址。

1. 盘点公网主机、域名、frps版本/证书/已有代理和端口。新版报告工作流使用前先升级远端控制端与Agent，同一提交验收后继续。
2. 增量添加隧道：原SSH保持，新明文转发端口先限制为仅回环可达；frpc校验frps证书。共享frps不能为本项目修改全局proxyBindAddr。
3. 添加公网HTTPS站点，先校验配置，再用客户端连接副本试用；通过后仅替换server地址，沿用已有token与数据。
4. 验收状态/身份/证书、实时日志、70份报告与制品、8MiB元数据、原SSH/节点、断开重启和回退；通过后更新正式部署状态。

部署前需填写：公网frps地址/控制端口、访问域名、证书/授权位置、既有服务标签与转发端口。默认复用现有中转机，当前未确认其具体配置。若暂时无公网域名，可规划私有CA入口，客户端须配置可信CA，不能通过跳过证书校验接入。

FRP默认TLS只加密，不默认验证frps证书，见[官方说明](https://gofrp.org/zh-cn/docs/features/common/network/network-tls/)。Caddy对SSE默认立即刷新，采用最小反代配置即可，见[官方反代文档](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy)。
