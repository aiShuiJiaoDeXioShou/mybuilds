# 002 验证指南

运行 go test ./...、go vet ./... 与 pipeline/CLI race 检查；构建客户端，在临时目录使用 examples/local-run.yml 执行真实 shell，检查显式参数/条件/post/UTC 日志/JSON，保留用户修改原 SHA-256。
运行超时与 SIGINT 的含后台子进程脚本，检查本组停止且无关进程存活。验证 secret 跨 Write/重叠/长行/输出错误、build 累计超时与 post 独立预算。
混合多 build，后一个含生效 upload/非法目录/缺失必要密钥时不得执行第一个。前一步替换工作目录链接时下一步拒绝越界。
Windows 客户端交叉编译只证明编译；dry-run 既有回归保留。真实结果记录 validation.md，再 converge。
