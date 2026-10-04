# 001 数据模型

- Document：Version=1；Builds 为完整名称集合；Notifications 为文件公共通知。原根级定义解析为 default，混写拒绝。
- Build：Runner、Params、Env、When、Timeout、Steps、Post、Reports、Notifications。所有 map 值仅指定类型，不接受隐式字符串转换。
- Parameter：Default *string 区分缺省与空值，Description、Required、Choices；简写字符串转 default。
- Step：Kind/Name 及各类专属字段；未命名生成 kind-位置；run 正文不插值，post 限 run/artifact；发布段顺序静态校验。
- When：Branches/Params/Changes；有效非空结构，标准路径 glob 语法（** 预留递归路径匹配），参数须声明。
- Preview：名称、参数名（值隐藏）、build/step 条件结果 ready/skipped/pending 和原因、kind/name、预算与非敏感摘要。
- 所有模型只表达本功能要验证的配置字段；数据持久化、节点归属、真实发布证据不在 001。
