# 003 验证与交接

2026-10-04，集成分支003-build-artifacts，前置已验收256af8e。specify/plan/tasks/analyze/implement已执行；13FR、5SC、三故事、五原则、九任务，analyze无阻塞。实现、集成检查与converge已通过，完整功能一次本地提交，不push。

## 实现与分区

A collector003的artifact.go/test/Unix test；B engine003的run.go/test；C logcli003的log.go/test、local_artifact_test.go、local-artifacts.yml。三者独立worktree基于256af8e，主代理冻结ArtifactRecord/ResultDir/LogPath与collector/logger接口、doublestar/v4 v4.10.2；只集成各自文件，未写stub或另一执行器。artifact只配置paths，复用build/post剩余预算，不新增timeout字段。
Root受限递归glob/实际文件访问；非阻塞打开避免FIFO字面目录前缀阻塞；每模式匹配后去重，源变化检测，完整stage/manifest后原子发布。静默run也保存固定安全system摘要；日志脱敏记录同时写终端与0600步骤文件，ResultDir在工作区外且0700。普通/post每步独立快照。

## 实际通过的检查

集成树：`go test ./...`通过（pipeline7.411s、client1.268s）；`go vet ./...`通过；`go test -race ./internal/pipeline ./internal/cli/client`通过（8.240s/4.285s）。新增与既有CLI检查通过；测试自身结果目录随测试清理。
A真实递归零层/多层、每模式零合法文件、重叠去重/同basename、manifest/hash/权限/源后改写、内/外/绝对/悬空链接/环/FIFO/socket、源复制期间取消与身份/大小/mtime变化、已有目标/并发目标、真实权限复制失败均通过；安全错误无原路径、失败无清单/临时stage残留。FIFO字面前缀先复现2s阻塞再修复。
B/C真实整批预检查、--step、参数模式渲染、when全跳过、1ns预算与failure诊断、其他build继续、post不覆盖、TMPDIR在源码内回退、原exit不被日志错误覆盖、终端/文件同UTC脱敏记录、分片secret/UTF8、两流、越界链接与实际写/关闭错误检查通过。

主代理构建`/tmp/mybuilds-mvp.zKtK0e/mybuilds-artifacts`，以`verify003.py`在自身临时仓库实际执行示例：核对普通/post四条副本内容与size/SHA-256、独立manifest、0700/0600、UTC两流/secret未出现、--step、零匹配失败日志/无manifest、dry-run/全跳过/后续build非法模式无结果目录和脚本；用户文件保持。脚本实际输出全部PASS，运行目录自行清理。
Windows amd64与Linux amd64客户端交叉编译通过，仅编译证据，不冒充实际Linux节点运行。

## 收敛与全局后续

旧002的30ms/150ms预算测试偶发CleanupFailed，已按独立speckit-bug-assess/fix/test真实复现并修复；在原窗口内复查瞬时EPERM，只有ESRCH确认停止。缺陷单独提交0937e83，原两预算50轮、新100轮30ms/活组确认、相关race与真实002二进制保护检查通过。003集成树修复后全量test/vet通过（pipeline10.385s），与全部未提交003文件隔离暂存。
后续Linux测试节点、签名工程/profile及商店应用/凭据路径已异步询问，尚未回复；当前003无需这些外部资源，不能将工具链存在当作后续真实验收。

003最终复验：完整pipeline/client race通过（13.540s/3.615s），最新二进制verify003再通过；converge核对13FR、5SC、10验收场景、9任务、6计划决策与5原则，missing/partial/contradicts/unrequested及各级严重度均0。收敛前后tasks SHA-256均fae60ef03072061bc0cd95387d106eb0f2463144b6d4bff44cdda297759a6411，无追加空phase；README及本地文档链接检查通过。
