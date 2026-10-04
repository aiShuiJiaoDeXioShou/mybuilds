# 初始化 CLI 契约

适用于 mybuilds 与 mybuilds-server。

| 操作 | stdout | 退出码 |
|---|---|---|
| 无参数、--help | 当前入口帮助，包含 version | 0 |
| version | 版本信息一行，默认见下文 | 0 |
| 不存在的子命令 | 不输出成功结果；stderr 报错 | 非零 |
| version 多余参数 | 不输出版本；stderr 报错 | 非零 |

版本格式：`dev (commit: unknown, built: unknown)` 加换行。
注入 Version、Commit、BuildDate 后按同一格式替换字段。
错误不重复输出 usage；标准 Cobra help/completion 可用，不注册业务占位子命令。
两端的帮助和版本不读取用户配置、不访问网络、不创建构建工作区。
