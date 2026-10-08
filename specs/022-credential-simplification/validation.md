# 验证记录

日期：2026-10-08；主工作区 `/Users/linghe/project/mybuilds`，分支 main，基线 `22e99ee`。

## Spec Kit

specify → plan → tasks → analyze → implement 已执行。需求质量 6/6；analyze 核对 5 FR、4 SC、两组场景及原则 I–V，无阻塞。extensions hooks 为空。implement → converge 已完成；5 FR、4 SC、5 AC、8 任务与原则 I–V 全部核对，所有计划文件归属/依赖/平台约束符合，无 missing/partial/contradicts/unrequested 缺口；converge 没有改写 tasks。

## 实际检查

| 检查 | 结果 |
|---|---|
| 修改前只读/延迟读取测试 | 8 个既有入口测试失败，确认原实现拒绝 0400 或提前读取秘密 |
| 修改后相同检查 | config/agent/scm 全部通过 |
| `go test -p 1 ./internal/agent ./internal/distribute ./internal/pipeline -run 'Test(Secrets\|IOSSecretRefs\|RestrictedMaterial\|Log)' -count=1` | 三包通过 |
| 相关五包 race（命令见 quickstart） | 全部通过，覆盖并发任务秘密/工作区/日志隔离、目录锁、读取权限及流式日志 |
| `go vet ./...` | 通过 |
| Windows/Linux amd64，CGO_ENABLED=0，三个 CLI 编译 | 6/6 通过；只证明可编译，不新增 Windows Agent 运行支持 |
| 本功能相对文档链接与 `git diff --check` | 7 项链接与空白检查通过 |
| `go test -p 1 ./...` | 首轮其余包均通过；client 包的旧诊断测试受本机现有连接配置影响失败，隔离后该包全部通过（见下） |

实际 HTTP/Git 夹具验证缺失或公开秘密文件：中央构建为 failed/precheck_error、步骤未开始、没有 checkout 工作区。既有公开权限、符号链接、FIFO、硬链接、重复键与大小限制继续验证。跨块/并发/长行日志算法未修改，既有测试通过。

## 行为与迁移

无需数据库/配置格式迁移。0400/0600 只读材料均可用，安装器仍按原规则生成私有配置；Agent 配置成功只保证秘密路径解析成功，任务及查询使用前验证内容。写入状态 0600、运行目录 0700 保持。没有新增依赖、防护级别开关或日志框架；未执行外部商店发布。

## 全量检查的环境差异

首轮唯一失败为 `TestAndroidDoctorJSONFailureAndOptionValidation`，它预期 `doctor --server` 失败，但当前宿主已有可用客户端连接配置，命令实际成功。该测试与 CLI 代码均未在本功能修改。使用 `MYBUILDS_CLIENT_TOKEN='' go test -p 1 ./internal/cli/client -count=1` 隔离宿主凭据后，整个 client 包通过（29.974s）；其他所有包在首轮通过（Agent 315.236s、server 83.210s、store 21.131s）。不改动实际部署或为满足旧测试退回远程能力。全量验收结论基于各包完整运行加失败包隔离重跑，并非把首轮非零退出记录为通过。
