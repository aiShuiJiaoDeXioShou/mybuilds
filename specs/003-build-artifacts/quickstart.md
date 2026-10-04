# 003 验收指南

go test ./...、go vet ./...；安全复制/预算/日志用例race。构建二进制，在临时仓库生成嵌套同名文件，以examples/local-artifacts.yml运行。解析JSON result_dir，核对每条实际副本的size和sha256、步骤日志的UTC/两流/secret脱敏；post重写源后普通副本保持原内容。
对每pattern零匹配、根内/外/绝对/悬空链接、目录链接环和特殊文件运行正反例；失败/取消时没有可见部分manifest，已有其他步骤快照不改变。先验证第二build非法模式导致全批无脚本/结果目录。--step只收集artifact，allskipped与dry-run无目录。
验证Windows/Linux客户端可编译，不冒充Linux实际节点运行。结果与收敛记录写validation.md后本地完整功能提交。
