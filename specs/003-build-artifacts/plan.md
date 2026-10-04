# 实施计划：003 产物与本地结果

**Branch**: 003-build-artifacts | **Date**: 2026-10-04 | **Spec**: [spec.md](spec.md)

## Summary
接入既有artifact分派，使用既定doublestar/v4递归glob和Go1.25 os.Root受限访问；本地每次独立结果目录，保存步骤快照及脱敏日志。不新增收集器接口、下载服务或发布模型。

## Technical Context
Go1.25，现有CLI/模型/执行引擎；仅新增锁定doublestar/v4 v4.10.2。GlobWalk使用Root.FS、NoFollow/FilesOnly/FailOnIOErrors，特殊文件仍须Stat过滤。ctx-aware FS检查取消，复制循环检查ctx、边复制边hash；完成步骤临时目录后原子rename，清单保存manifest.json。普通/post及不同step独立路径。
本地Run在全批预检查后且有生效选中步骤时才创建0700临时结果根，必须在工作区外；Root持有句柄，日志/产物0600。日志已有脱敏器写出的同一记录同时保存logs/<build>/<step>.log，Root下访问，Run负责最终关闭及错误传播。

## Constitution Check
五原则设计前/后通过：完整SpecKit与可验收范围；共享pipeline无空模块；只增加既定递归glob库；Root/原子快照/秘密与取消边界；中文与真实验证。Root不防同用户任意恶意shell，不把可信脚本当作沙箱。

## Project Structure 与归属
- 主代理：internal/pipeline/run_types.go、go.mod/go.sum、specs/003、README、历史、专题必要澄清、集成。
- A：internal/pipeline/artifact.go、artifact_test.go（Unix特殊文件用例可放artifact_unix_test.go）；collector、模式校验及受限复制/manifest。
- B：internal/pipeline/run.go、run_test.go；准备模式、结果目录、artifact执行/预算/post/日志路径、根句柄生命周期，唯一执行器写入者。
- C：internal/pipeline/log.go、log_test.go；日志镜像Root、文件管理/关闭；internal/cli/client/local_artifact_test.go、examples/local-artifacts.yml。不改CLI入口或加未要求flag，现有JSON自动序列化新字段。
接口见contracts/go-api.md，外部行为见contracts/cli.md。

## 阶段与依赖
以已验收256af8e建立三个独立worktree，主代理冻结类型/依赖/文档。A/C并行，B按冻结接口实现、实际同步A/C后验证；不写stub。主代理串行整合、全量test/vet及针对安全/取消/日志race、真实二进制快照/post/hash检查，再converge与功能提交。

## Complexity Tracking
具体函数与既有执行路径，无collector interface、hook总线或另建存储服务。Root替代检查后普通Open，库替代手写**匹配。每模式独立匹配再去重。
