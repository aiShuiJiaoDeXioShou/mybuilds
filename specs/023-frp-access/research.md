# FRP 接入研究

2026-10-09；只读研究，无远端配置修改。

| 决定 | 原因 | 考虑过的替代 |
|---|---|---|
| 外部TCP转发，公网Caddy HTTPS | 沿现有HTTP接口透传，应用无新依赖 | FRP内嵌Go、应用管理证书增加生命周期工作 |
| frpc/frps TLS校验证书 | 默认加密不等于验证身份 | 默认TLS不足；不再叠加useEncryption |
| 共享frps保留proxyBindAddr | 无独立bindAddr，改全局可能中断SSH | 独占可回环绑定；共享仅限制新增端口 |
| Caddy保持流式默认 | text/event-stream立即flush | 不加flush_interval -1，其行为还影响客户端断连传播 |
| 先验副本，再切客户端 | 旧SSH可回退，同机Agent无需迁移 | 不改控制端listen或全部节点配置 |

frpc0.64.0的[TOML示例](https://github.com/fatedier/frp/blob/v0.64.0/conf/frpc_full_example.toml)支持环境变量、TLS可信CA和serverName。默认TLS不验证服务器证书，需配置信任，见[FRP TLS文档](https://gofrp.org/zh-cn/docs/features/common/network/network-tls/)。

TCP监听使用服务端全局ProxyBindAddr，见[0.64.0监听实现](https://github.com/fatedier/frp/blob/v0.64.0/server/proxy/tcp.go)；公网暴露测试覆盖IPv4/IPv6。

Caddy默认透传Authorization、方法、路径和查询参数，SSE立即刷新，且默认不启用上游重试，见[reverse_proxy文档](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy)。不增加BasicAuth、请求缓冲或更小body限额。

公网主机、域名和证书路径是环境参数，技术路线已确定。尚未盘点公网frps/Caddy，不声称与其实际版本完成连接验证。
