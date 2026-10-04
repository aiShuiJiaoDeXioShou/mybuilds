# 002 运行模型

RunOptions 复用 PreviewOptions（Names/All/Params/Step/Facts），增加 Workspace（默认当前目录）和 Output（默认 io.Discard）。RunResult 为有序 Builds；每个 BuildRun 保存 name/status/reason/duration_ms/steps/post。状态 succeeded/failed/cancelled/skipped，post 错误单独记录。
StepRun 保存 name/kind/status/reason/exit_code/duration_ms，无脚本、环境值、密钥或原始 error。shellCommand 内部保存 Path/Args/Dir/Env，不序列化；shellResult 为 Started/ExitCode/Reason/Duration/CleanupFailed。
使用 validationCopy 避免 Validate 补名修改调用者。全部选择预检查通过才启动第一步，每步启动前复检真实目录边界。原结果冻结后才开始 post。
