# 自定义发布示例

这是可信仓库脚本示例，不是系统内置接收服务。将 `mybuilds.yml` 的三步复制到仓库根配置，并把脚本纳入同一 Git 提交；节点须有 `python3`，明确 secrets 文件声明 `CUSTOM_ENDPOINT` 为自有 HTTPS 接收端根 URL。接收端以原 `intent_id` 为路径和记录键：POST 接收原二进制，GET 只读取该次原记录。示例禁重定向、没有 POST 重试、使用默认 TLS 验证，最大包16MiB；不安装工具、不访问商店。

上传和查询请求的 `X-Mybuilds-Metadata` 包含原 ID、授权摘要、版本及产物元数据，不含私有路径。服务须按 [custom 结果协议](../../specs/012-custom-workflows/contracts/custom-publish.md) 返回≤64KiB的精确 JSON，包括原身份、摘要、`status`、固定 `evidence_code`、`remote_id` 和真实 `action_confirmed`；HTTP 200 本身不能证明发布成功。查询输入没有 `artifact.path`，脚本不读取或重新上传旧包。

管理员先用自有0600 verification-file的 `manual_attested` / `ownership_attested` 依据绑定 `org.example.custom`，再显式 `--allow-upload` 触发。没有应用归属、产物/报告依据或有效执行权不会获得授权。系统只授一次命令；未知结果保留应用保护，不能借重试构建自动重发。用户脚本自身及接收端仍是信任边界，系统不证明其内部网络恰好一次或 query 绝无副作用。

此示例只作可编辑案例，未对用户接收端或商店执行人工验收；自动门使用独立自有接收端与真实脚本进程。
