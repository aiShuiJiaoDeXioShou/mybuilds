# 安装文件

安装程序：用户bin目录；配置：client.yml/server.yml/agent.yml；管理身份：server安装目录的client.yml；节点身份：agent.yml；日志：配置目录logs。均复用现有字段。
安装完成标识保存版本、角色、路径；重跑必须一致，版本变更提示维护步骤而不自动替换运行程序。
发行包：mybuilds_VERSION_OS_ARCH.tar.gz（Unix）或.zip（Windows）与SHA256SUMS。
