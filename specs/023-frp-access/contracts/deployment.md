# 部署契约与示例

域名、端口与证书路径均为示例，按盘点填写。FRP凭据与mybuilds管理员/节点凭据独立，不提交明文token。

## 控制端主机

server.yml保留`listen: 127.0.0.1:8787`。独立frpc配置：

```toml
serverAddr = "frp.example.com"
serverPort = 7000
loginFailExit = false
auth.method = "token"
auth.token = "{{ .Envs.MYBUILDS_FRP_TOKEN }}"
transport.tls.enable = true
transport.tls.trustedCaFile = "/path/to/frps-ca.pem"
transport.tls.serverName = "frp.example.com"

[[proxies]]
name = "mybuilds-control"
type = "tcp"
localIP = "127.0.0.1"
localPort = 8787
remotePort = 18787
```

MYBUILDS_FRP_TOKEN由现有秘密管理或私有服务配置注入；运行账户能读CA。`frpc verify -c ~/.mybuilds/frpc.toml`只验证配置，不证明连通。独立系统服务托管frpc，mybuilds不启动它。

## 公网主机

复用frps保留bindPort/auth和原代理许可，检查allowPorts包含新端口。证书使用实际信任链，以下片段不可覆盖整份现有配置：

```toml
# 仅示意frps所需TLS字段
transport.tls.certFile = "/path/to/frps.crt"
transport.tls.keyFile = "/path/to/frps.key"
```

共享frps保持原proxyBindAddr及tls.force设置；只有确认全部旧客户端均使用TLS时才可改为`transport.tls.force = true`。先由主机防火墙仅允许回环到新18787端口，覆盖IPv4/IPv6，验证后才能启动代理；不能只检查云安全组就宣称不可达。独占frps才可全局设`proxyBindAddr = "127.0.0.1"`并强制TLS。

公网Caddy增量站点：

```caddyfile
builds.example.com {
    reverse_proxy 127.0.0.1:18787
}
```

先执行`caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile`，再按已有服务方式reload。域名证书准备后只使用HTTPS；不启用整请求/响应缓冲，不缩小body限额，不增加业务重试。80/443已有代理时沿现有站点机制接入，不抢占端口。

Authorization、方法、路径和查询参数保持，原身份校验生效。公网TLS与FRP隧道TLS各自校验。Caddy默认SSE立即刷新、默认不重试，见[官方文档](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy)。

## 客户端与节点

复制原client.yml为私有验证配置，仅把server替换为`https://builds.example.com`，保留token/timeout。私有CA才填ca_file。先用`mybuilds --config /path/to/test-client.yml status --json`验证，再替换实际地址。

同机Agent保留`http://127.0.0.1:8787`；跨机Agent更改server前先drain并等待无任务，原token/node/data_dir保持。旧SSH隧道留到正式验收通过。
