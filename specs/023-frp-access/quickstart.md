# FRP 真实验收指南（尚未执行）

准备公网主机、访问域名、frps证书与授权、匹配版本frpc、已有控制端/Agent；按[部署契约](contracts/deployment.md)填写参数，记录原服务与SSH路径。新版Java工作流先升级控制端/Agent到同一提交，不仅升级客户端。

1. 校验frpc/Caddy配置，检查新转发端口已限制为回环，原SSH仍通，再加载新增配置。
2. 用私有连接副本执行`mybuilds --config /path/to/test-client.yml status --json`，查询成功且项目/节点数保持。错误token、错误CA或域名证书均应失败，不打印token。
3. 授权独立测试项目生成70份JUnit和一个制品并触发普通构建；`logs BUILD_ID --follow`在结束前持续得到日志，详情含70份报告。下载逐份XML/制品并核对Size/SHA-256，测试生成报告与真实Java案例分开记录。
4. 验证合法大于1MiB且不超过8MiB报告事件经代理接受，超应用上限拒绝；实际上传/下载较大制品验证流式传输，不能用配置代替结果。
5. 独立外网检查18787明文端口IPv4/IPv6均不可用、HTTPS443可用；原SSH、同机Agent、项目状态通过。
6. 仅用独立测试构建验证frpc断开/重启；不为测试中断用户任务或删除unknown journal，原停止/未知副作用保护保持。
7. 回退client.yml到备份并确认原SSH查询成功，再移除本次新增代理/站点及专用端口规则，记录启停；不删除其他代理。

全部门通过后speckit-converge、保存validation.md并一次本地功能提交。当前规划交付不勾选真实部署门。
