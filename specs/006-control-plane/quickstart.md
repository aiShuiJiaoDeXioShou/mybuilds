# 006 实施后真实验收指南

本指南是待实现后的验收步骤，不表示当前命令已存在。基线2ab8991、真实Go1.25.4/Git；控制端macOS/Linux。只使用自有临时数据/仓库/数据库，所有脚本只用于证明控制端没有执行，不扫描用户仓库或签名资源。

## 本机服务与远程CLI

1. 在工作区外创建0700临时data_dir与自有Git仓库，提交合法多build流水线（含runner或使用项目default_node）、两个不同提交及禁止执行的写文件脚本。
2. 编译cmd/mybuilds与cmd/mybuilds-server到临时bin；按config-cli创建server.yml，数据库指自有临时普通SQLite文件。
3. 首次serve前仅通过MYBUILDS_BOOTSTRAP_ADMIN_TOKEN提供自有高熵测试身份；不放argv/YAML/日志。启动serve后创建0600 client.yml：server为本机URL，token为`${MYBUILDS_CLIENT_TOKEN}`；通过环境提供同一测试身份。
4. 用客户端group create、project init（明确--repo、--nodes、--default-node、--group）、trigger --build或--all、build ls/show --json、status --json验证；本机serve在线时另一个migrate/project/token写命令应独占失败，改用remote入口。
5. 参数使用共享`--param version=1.2.0`与命名`--param android:channel=internal`；when不匹配记录skipped/null号，模板缺number/node/workspace的build仍可queued。检查脚本marker从未创建。
6. 退出serve后本机管理/重启成功，历史/编号/身份/条件/预算/步骤进度不变。bootstrap撤销后再带原环境重启不得复活。

## 同一业务suite跑双数据库

主代理准备的真实PG16.14夹具：`/tmp/mybuilds-mvp.zKtK0e/postgres006-ldh7rr0w/fixture.json`；socket-only自有trust测试库，不包含生产秘密。具体测试DSN由MYBUILDS_TEST_POSTGRES_DSN提供，不写命令行或公开结果。测试用唯一自有schema/数据库，串行owner或独立namespace，不能相互drop；PG服务最终stop由主代理负责。

实施后执行标准go test ./...、go vet ./...与相关race。store双驱动测试必须在提供该DSN时运行实际PG，未提供时明确报告待真实验证，不能将DryRun或静默skip作为SC-001通过。记录sqlite_version()=3.53.3、每连接WAL/FK/busy设置以及实际PG server_version。

共同验收：CRUD/default/迁移/历史查询/空组删除竞态；20并发同key只一批、20不同key号不重复；中途DB约束或编号溢出回滚全部；全skipped不消耗编号；分页稳定且project/group交集；token摘要/撤销/bootstrap sticky。

## 独占与失锁

- 两个真实mybuilds-server进程竞争SQLite规范化同文件及PG同数据库不同DSN；第二个拒绝，原关闭后可启动。SQLite symlink路径归同一identity，hardlink数据库拒绝。
- 替换仅本次自有SQLite锁文件、或仅终止本次自有PG backend_pid，原运行权丢失后写请求必须拒绝且计数/批次不变；不能重连旧运行权。
- 原型已有PG session取得/第二session拒绝/终止第一/旧连接失败/新session取得5项证据，位置`postgres006-ldh7rr0w/advisory-prototype.json`；功能实际Store行为仍须独立验收。

## 固定提交与安全负例

真实Git：分支HEAD推进后同key原请求保持原SHA/snapshot/编号；新key读取新提交；完整可达ref成功，tag/短SHA/不可达/越权ref失败。配置缺失/坏类型/未知字段/文件symlink/submodule/目录/路径越界/非法glob/超限无入队与marker；自有恶意reference-transaction hook、template/global config不执行。

HTTP/CLI：三角色、缺token/错误/撤销身份；所选upload即使when=false仍只有admin+allow_upload可入队且绝无实际发布；trigger/approver管理越权拒绝；重复字段、多对象、超限、错误page与unknown配置报固定错误。用唯一测试敏感标记检查stdout/stderr/HTTP/普通日志无参数值/密钥/DSN/repo密码/脚本正文；TokenCreate唯一一次授权结果是例外。

本地init/run/doctor/help/version在损坏client.yml与缺token下保持既有行为，remote --timeout不改本地YAML预算。Linux/Windows客户端CGO=0跨编译，控制端服务Windows实际执行明确不支持。

## 完成记录

validation.md记录日期、提交、工具/DB版本、脱敏命令与真实结果，不把研究原型算功能验收。主代理完成全量/converge并一次提交。005/009/全MVP真实签名门保持，006只声明控制面排队；005未来集成后再复验ios_signing快照往返和005+007真实租约/清理联验。
