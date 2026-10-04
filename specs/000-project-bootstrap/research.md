# 初始化选型记录

- **Decision**: 使用本地 module mybuilds、Go 1.25、双 cmd 入口。
  **Rationale**: 远端未配置；应用无需对外提供库，与已确认架构一致。
  **Alternatives considered**: 不编造 GitHub 模块路径；不拆多个 Go 模块。
- **Decision**: 仅引入规划中的 Cobra v1.10.2。
  **Rationale**: 后续命令树已确定使用 Cobra；go list -m -json github.com/spf13/cobra@latest 实测为此版本。
  **Alternatives considered**: 标准库 flag 适合简单参数，但会重复后续 CLI 框架迁移；YAML/ORM/SDK 此时没有调用场景。
- **Decision**: 共享版本字段与命令，分开客户端/服务端 root，未来服务生命周期与调度合并 internal/server。
  **Rationale**: 两个入口已有实际复用需求；避免 serve/queue 互相协调生命周期。
  **Alternatives considered**: 不创建通用工厂、接口层与空业务包。
- **Decision**: README 只列实际运行命令，目标目录标注按功能创建。
  **Rationale**: 当前没有构建、数据库、审批或通知实现；保持概览可验证。
  **Alternatives considered**: 不提前生成假流水线、空部署文件或全部依赖。

研究依据：项目 PLAN、AGENTS、实际 Go/Spec Kit 环境及并行只读发现。无待澄清选型。
