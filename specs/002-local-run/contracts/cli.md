# 002 CLI 契约

无 --dry-run 时调用 Run；日志 stderr，脱敏结果 JSON stdout。预检查失败无结果/副作用；执行失败保留 JSON 且非零退出，SIGINT/SIGTERM 取消本批次。既有参数及 dry-run 行为保留。
--file 只选配置，工作区仍为当前目录。--step 仅单 build 的指定普通步骤，不自动运行前序依赖，但仍检查完整选择集合与发布限制。
没有本地 --version/--channel 快捷参数，规划仅在 trigger 提供。生效 artifact/approval/reports/notifications 后续接入前明确未支持（有效通知 enabled:false 可禁用）；upload 由远程执行。runner 目前仅核对宿主，doctor/模板后续交付。
