# 001 验证指南

在项目根目录：

```bash
go test ./...
go vet ./...
go build -o /tmp/mybuilds-preview ./cmd/mybuilds
```

在临时目录使用该二进制 init；生成配置后 run --dry-run，输出 default 和 run 步骤且不产生脚本副作用。重复 init 应失败且原文件 SHA-256 不变。
用 examples/pipeline-preview.yml 分别 --build android 和 --all 预览；省略选择、未知 build、--all/--build 混用、重复/未知参数应失败。
配置副作用脚本 touch marker，预览后 marker 不存在；credentials/Webhook/环境引用和 run 正文均不回显。未定义模板变量拒绝且不回显旁边的敏感字面量。
正常 run（没有 --dry-run）、平台模板选项暂未实现，返回非零且无外部动作。go test 保持已有双入口帮助与版本用例。
验收证据写 validation.md；这是自动行为验证，不需要移动端工具或商店账号。
