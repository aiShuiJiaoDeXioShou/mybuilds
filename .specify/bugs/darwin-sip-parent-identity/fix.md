# Bug Fix：SIP未知对象原父身份核对

- **Slug**: darwin-sip-parent-identity（明确取刚完成assessment）
- **Fixed**: 2026-10-04T18:21:00Z
- **Assessment**: ./assessment.md
- **Status**: applied

## Summary

按原父kernel identity区分init真实创建与被重新父化的PPID1对象，不按名称放行。仅unreadable消费者使用固定56B proc_info、精确init unique与原父version、再次身份与Kinfo birth/父/trace复核；失败仍unknown/noSignal。

## Changes

| File | Change | Notes |
|---|---|---|
| internal/process/scope_darwin.go | 修改 | 具体darwinIdentity/darwinBornToInit消费，标准syscall及固定byte buffer，Linux/public API/依赖不改 |
| internal/process/scope_darwin_test.go | 修改 | 真实重新父化对象原父version不同init，不能排除；原SIP/并行门保留 |

## Tests Added or Updated

真实fork→/bin/sleep exec→父退出PPID1，核对实际原父仍不同init；已记录orphan两次原身份一致且current PPID1。正常Git HTTPS TLS错误不得附加虚假CleanupFailed，真实原有SCM测试消费。

## Local Verification

标准syscall self实际返回56/errno0/非零unique；init unique1/version610。真实SIP orphan两次unique3229649/parent3229648/idversion6120260/originalparent6120258，PPID1实际确认，原父version不等init610。无需cgo或新依赖，临时安全探针只自有spawn与清理，无系统服务kill。目标process/SCM race进行中。

## Deviations from Assessment

无文件范围扩大。未把私有ABI头文件存在当支持，先实际syscall与真实重父probe；原父version有限位数风险与Darwin信号TOCTOU保持，如有不可证反例仍停止修正。

## Follow-ups

待目标门、完整回归与双库三入口行为通过；原detached-gradle-stop仍等待207正常签名完整门。两项均未提交，未解除旧204保护。

补充：父先Gone后exec的真实负门已实现并通过race4.049s；实际init parentunique不能替代原父version。32位版本理论回绕与init exec误拒限界见test.md，不宣称永不碰撞。
