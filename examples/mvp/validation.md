# 集中案例验证记录

本页区分已经执行的自动检查与用户待执行的真实签名、商店验收。操作入口见[验收指南](acceptance.md)。

| 已执行检查 | 结果 |
|---|---|
| 真实CLI init生成单YAML的Android/iOS案例，internal/store参数dry-run | 通过；仅检查配置，不声称完成移动构建 |
| Flutter3.38.6/Dart3.10.7实际pub get及widget测试 | 通过；2026-10-05实际命令退出0，1个测试通过，生成真实命令聚合JUnit |
| generic辅助脚本实际CLI/HTTP/HTTPS/HMAC/重复ID/私有文件与URL边界 | 16项通过；自有进程已wait退出 |
| Webhook三二进制实际等待窗口/去重/原SHA/changes/Node Run | SQLite与PostgreSQL各34项，共68项通过；模块合并目标8包通过 |
| 审批实际暂停/HTTP批准/同节点同Attempt新epoch继续/原报告复用 | 已通过；原工作区不重复checkout，原XML不重复上传，最终报告seal与post正常 |
| custom实际原意图/单次命令/metadata query及未知Close保护 | 模块与最终双库三CLI联合检查通过 |

真实Flutter测试JUnit SHA-256：`43ec47724ba3a5508711b70096e8797390f6f81a690a7959494a525ff4132961`。该XML记录实际命令的聚合结果，不伪造Flutter逐case报告。辅助Hook脚本证据SHA-256：`56f17ce9f22e6f5b2e5f5e57a7424252aaa0bd22ec6d376f024fca6f1dcf4d70`。

## 人工待验

合法原生/Flutter双平台签名、实际Google Play internal上传、ASC上传与明确App Review提交、真实外部Git托管push及重启故障场景按指南逐项登记。记录应用/版本/build编号、原SHA与报告/制品摘要、审批/意图/远端标识和通过或失败结果，不保存私有密钥或token。

## 最终MVP集成

16个MVP代码模块已交付，必要自动检查与收敛完成；真实签名、商店与外部provider仍人工待验。最后整项目普通检查发现的真实缺陷和旧断言已修复，并只复验受影响范围；原失败日志保留。修后小包完整normal、审批/报告/Hook双库race、最后五包受影响race、whole vet和18次编译/6入口检查通过。

最终实际三二进制、自有Git/可复用profile与验证HTTPS的custom接收器，SQLite/PostgreSQL各手动/自动两种触发共4个构建，118/118检查通过（2026-10-05T05:24:10.618210Z–05:24:57.367071Z）。每构建两次明确批准、同attempt epoch3、上传一次、最后always真实执行；自动待审批复投只复用原ID且不占号，手动不被合并。原binary/XML下载、旧Grant精确只读lookup、metadata GET不改终态通过；8自有服务均wait退出，源字节前后一致。摘要见[evidence.json](evidence.json)。

原完整证据SHA256 `0c1922150daeab9af079a46cdaac7557affdb7a7b71221ea1a8cb50760ca5eeb`；脱敏清单`fb80c40c246f199c2ae43a43ade4643a6caa3a4c8da7175f6277af3e0bda89ff`。本案例是实际自有custom发布，不代表Apple/Google的真实商店上传或四平台外部push。
