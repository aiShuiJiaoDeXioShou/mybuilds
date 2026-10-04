# 实施计划：008 重启核对与原快照重试

**分支**：`008-build-recovery` | **日期**：2026-10-04 | **规范**：[spec.md](spec.md)

**输入**：已冻结的19FR、5SC、13AC；前置007已验收并提交 `85b46bfaccb45c4626fcffbbcb526b2f7f8c9301`。

## 摘要

控制端持唯一写锁，在监听前核对持久队列和执行记录；有效租约沿原身份继续，到期仍按007中断并保留停止保护。Agent原进程在保守执行期限内抵抗短暂控制端断连，新进程只核对精确已确认终态回执，不接续旧动作。显式retry以独立短事务复制原快照，重新核对当前授权，分配新号并关联原构建。全部实际动作继续调用既有Checkout、Run与process.Run。

## 技术上下文

- **语言**：Go 1.25.0，单模块；注释与文档中文。
- **依赖**：现有标准库HTTP/context/os.Root、Cobra、GORM、SQLite驱动、pgx/PostgreSQL、UUID、x/sys；不新增或升级依赖。
- **持久化**：沿006/007 SQLite与PostgreSQL模型、Store.write短事务、独占控制端锁及Agent私有0700目录/0600journal。
- **测试**：真实Go行为测试、race/vet、子进程CLI/Server/Agent、独立双库相同suite；macOS与Linux真实节点和受控HTTPS网络。
- **目标**：Linux/macOS控制端和Agent，客户端三入口原交叉编译矩阵；005签名仍未交付，Linux ARM不冒报Android工具能力。
- **规模与限额**：沿现有1..32节点容量、服务端并发、128条本地journal/单条1MiB及有界HTTP；20次同retry请求实际竞争只产生一个新号。恢复核对使用有界ctx和分页，不把所有历史日志/文件装入内存。
- **性能边界**：不新增吞吐承诺；启动门未完成不接入调度，事务不等待Git、网络、进程或文件流。网络等待仍计既有纳秒活跃预算，不能增长或重置。

## 原则检查

Phase0前与Phase1后均PASS（constitution 2.1.0）。I：仅完成plan，不提前tasks/代码/提交，后续analyze/implement/converge由根推进；II：沿三入口、单控制端、多节点及单一Run；III：无新依赖、执行器、repository接口、事件框架或未来状态；IV：当前独立身份、精确fence、停止未知保护、快照/鉴权与编号同事务，日志秘密沿原边界；V：规划真实双库/两平台与失败证据，准备不计验收。无原则例外。

## Phase0研究结论

见 [research.md](research.md)。已核实007源码：启动已调用ExpireLeases；事件收据目前只存seq/digest；Agent拒所有旧journal；retry直接调用Enqueue会重写Facts/授权；Repository登记后不可变，项目被历史构建FK保护，故无需新仓库迁移字段。无NEEDS CLARIFICATION。

## Phase1设计与实施顺序

1. 根冻结 [Go契约](contracts/go-api.md) 的最小共享类型；C增加retry关联/终态收据证据与迁移真实检查，先双库红绿。
2. C实现启动Recover与Retry；根接监听前恢复门、真实鉴权retry路由/CLI及安全DTO。先完成重启有效/queued与幂等retry的可运行交接。
3. C提供具体只读TerminalReceipt；根接当前节点鉴权HTTP；B在持本地锁、新session之前受限核对自有journal。缺失/冲突/未知都保留，不能通过自动执行或旧PID信号解决。
4. B收窄网络错误分类，心跳/续租和当前合法普通回执在既有期限内重试；终态不能重放，丢ACK走只读确认。实际到期或失权仍立即闭锁全部post。
5. 根串行集成及双库同suite、macOS/Linux实机重启/失联/retry/终态ACK丢失，保留全量test/race/vet/构建门；完成收敛后一次整功能本地提交。

详细状态和迁移见 [data-model.md](data-model.md)，HTTP/CLI见 [contracts/http-cli.md](contracts/http-cli.md)，门与证据见 [quickstart.md](quickstart.md)（validation.md由根记录真实交接/实现结果）。019只协同原报告定义深拷贝、新attempt证据空、已确认封存报告不重解析；当前reports仍unsupported，不预建报告状态。

## 覆盖与实际门

| 故事 / AC数 | FR覆盖 | SC与预定实际门 |
|---|---|---|
| US1 / 3 | 001–005 | SC001/005：双库实际重启，queued+valid lease原号/身份/NS，锁丢失与损坏启动拒绝 |
| US2 / 3 | 004–007、013 | SC002/004/005：ordinary/always各失联与真实组回收，停止未知保持guard，旧fence拒绝 |
| US3 / 3 | 008–013、015、017 | SC003/004/005：原SHA/定义/最终参数/静态条件，新号，20重复/响应丢失与权限负例 |
| US4 / 4 | 006、014–019 | SC004/005：安全关联查询、只读terminal receipt正负例、两平台实际验证与后置边界 |

## 目录与所有权

```text
specs/008-build-recovery/
  spec.md / checklists/requirements.md       # 已冻结，规划不改
  plan.md / research.md / data-model.md
  contracts/go-api.md / contracts/protocol.md / contracts/http-cli.md
  quickstart.md / validation.md
internal/store/                             # C：模型、事务、迁移与双库测试
internal/agent/                             # B：journal核对、网络期限与实际Agent测试
internal/server/ + internal/cli/client/     # 根：恢复启动、节点/用户路由、retry命令及测试
internal/protocol/                         # 根唯一写：终态回执消息
```

分区实施前由根建立基于已验收007的独立worktree。B不新增pipeline/process执行路径；若行为检查证明现有Run必须调整，先向根报告具体消费者并串行划定文件。config、deps、README、产品文档、历史和验证记录由根唯一维护；共享文件逐批SHA核对同步后复验，不能跨分区并写。tasks由下一阶段生成。

## 复杂度记录

无违例，无新增框架。新增两个持久字段用于精确终态证据，一个构建关联字段，以及三个有实际消费者的Store方法；继续使用原表、进度/日志/产物流和停止确认入口。


## 实测补充：终态与续租交接

真实Linux三入口重启门发现中央已提交完整terminal但Agent续租worker随后收到租约结束409，可能取消正在读取的terminal ACK而退出，导致后续queued等候。沿唯一taskExecution，在发送build_finished前停止本次renew worker并等待当前真实请求完成，随后再次检查原Authority/数据锁；不cancel执行权或重置deadline、不忽略永久fence，不发第二次terminal。等待仍受原Authority期限；未知失败保PendingEvent与既有stop-confirm规则。此处是FR003/005/018/019及SC001/005的实际接线，不新增API/模型/执行器；只由root补已释放的008Agent execute/lease及真实测试。
