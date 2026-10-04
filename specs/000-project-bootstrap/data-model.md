# 初始化数据模型

本功能没有数据库或持久化业务实体。

## 构建版本信息

| 字段 | 默认值 | 约束 |
|---|---|---|
| Version | dev | 构建时可由 Go ldflags 注入 |
| Commit | unknown | 构建时可注入提交标识 |
| BuildDate | unknown | 构建时可注入构建时间 |

两端通过同一包读取，输出格式见 [CLI 契约](contracts/cli.md)。无状态迁移。
