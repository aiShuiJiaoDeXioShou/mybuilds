# FRP 接入实施任务

输入：[spec.md](spec.md)、[plan.md](plan.md)、research/data-model/contracts/quickstart。此次仅规划，以下均为后续真实部署任务，不把静态配置检查当作已部署。

## Phase 1：Setup

- [ ] T001 盘点公网主机/域名、frps版本/原代理/证书、80/443/18787占用和服务标签，填入 specs/023-frp-access/contracts/deployment.md 并建立私有部署配置，记录凭据位置而非token。（FR001/005/007）

## Phase 2：前置条件

- [ ] T002 drain远端节点、等待任务结束，备份程序/配置/数据并按 docs/INSTALL.md 升级控制端与Agent到同一提交；用原SSH查询状态/节点，保存 specs/023-frp-access/validation.md，不重置身份或数据。（FR008）
- [ ] T003 按 specs/023-frp-access/contracts/deployment.md 确认frps证书信任、保持原SSH代理和全局绑定；先在公网主机限制新增明文端口仅回环可达，覆盖IPv4/IPv6，保存外网验证记录后才可启用代理。（FR002/003/005；SC003）

## Phase 3：US1 公网访问（P1）

目标：独立客户端经HTTPS完成同一控制端普通构建和结果查询。独立验收：quickstart第2–5步。

- [ ] T004 [US1] 在控制端用户 ~/.mybuilds/frpc.toml 填入独立name、原回环8787、已隔离remotePort与可信CA/serverName；先verify再按原服务管理方式托管，保持 server.yml 的回环监听。（T003；FR001/002/003）
- [ ] T005 [US1] 按公网主机 /etc/caddy/Caddyfile 的既有站点机制增量配置域名反代，先validate再加载；保持Authorization/方法/路径/流式默认、不缩小容量、不增加请求重试，保存证书与配置版本。（T004；FR002/004/007）
- [ ] T006 [US1] 从客户端 ~/.mybuilds/client.yml 建立私有验证副本，只改HTTPS server，验证正确/错误token及CA；通过后切实际客户端，保留同机Agent回环配置、跨机Agent按节点独立验证。（T005；FR002/003；SC001）
- [ ] T007 [US1] 逐项执行 specs/023-frp-access/quickstart.md 第3–5步，真实70报告/制品/SSE/大消息和外网端口边界全部记录于 validation.md，核对每份Size/SHA，不以生成测试替代真实Java案例。（T006；FR004；SC002/003）

## Phase 4：US2 恢复与回退（P1）

目标：原连接/其他代理保持，新增隧道可恢复或撤回。独立验收：quickstart第5–7步。

- [ ] T008 [US2] 在独立测试构建中停止/重启新增frpc服务，验证不重放已开始动作、原租约/未知结果保护保持，复查原SSH和同机节点，保存 specs/023-frp-access/validation.md。（T004；FR005/006；SC003/004）
- [ ] T009 [US2] 按 specs/023-frp-access/quickstart.md 回退连接备份及仅本次代理/站点/专用端口规则，验证旧SSH查询、数据与身份保持；记录恢复到新入口或保留旧入口的最终状态。（T007/008；FR007；SC004）

## Phase 5：文档与整功能交付

- [ ] T010 同步 README.md、docs/INSTALL.md、docs/plans/FRP_ACCESS.md 和实施历史的真实状态；若修改目标Mac mini服务，调用其registry技能同步实际服务注册表，不记规划为已部署。（T007–009；FR007）
- [ ] T011 按实际验收记录执行speckit-converge，缺口继续implement；无缺口后仅提交本功能代码/部署文档/规范/验证一次本地提交，不自动push。（T010；FR007）

## 依赖与文件归属

T001→T002→T003→T004→T005→T006→T007；T008可在T004后独立验其停止/重启与原SSH，最终T009等待两故事结果。T010→T011收尾。系统配置和共享文档由主代理串行写；只读证书/端口研究可并行，不并发改共享frps/Caddy。

## 需求覆盖

| 需求 | 任务 |
|---|---|
| FR001 | T001/T004 |
| FR002 | T003/T004/T005/T006 |
| FR003 | T003/T004/T006 |
| FR004 | T005/T007 |
| FR005 | T001/T003/T008 |
| FR006 | T008 |
| FR007 | T001/T005/T009/T010/T011 |
| FR008 | T002 |
| SC001 | T006 |
| SC002 | T007 |
| SC003 | T003/T007/T008 |
| SC004 | T008/T009 |

## 实施策略

先完成版本与传输门，再试用公网地址；US1通过仍须验收US2才能正式替换旧入口。此功能无Go源码任务，检查以真实部署为主；若验收暴露产品缺陷，走独立bug流程，不通过修改测试掩盖。
