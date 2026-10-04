# 003 模型

ArtifactRecord：SourcePath（仓库相对/分隔符为/）、SnapshotPath（结果根相对）、Size（实际复制int64）、SHA256（hex）。A collector返回的SnapshotPath先相对步骤目标目录，B添加步骤前缀成为结果根相对路径。
RunResult增加ResultDir，StepRun增加Artifacts和LogPath；原状态/原因/私有started不变。日志位于logs/<build>/<step>.log，快照位于artifacts/<build>/<step>/files/<source>，manifest位于同step目录。
每次Run临时结果目录权限0700，文件0600，多个步骤永不复用目标。结果目录已存在文件不覆盖；无效输入/dry-run/allskipped无目录。Root由Run打开/关闭，logger只关闭其文件。
