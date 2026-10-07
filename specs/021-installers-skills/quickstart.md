# 验收

运行 python3 scripts/release.py --version v0.1.0 --output /tmp/mybuilds-release 生成本平台或指定平台发行包；python3 scripts/test-install.py 执行隔离安装与失败保护检查。
下载包解压后 bash scripts/install.sh server --service none --with-agent --bin-dir TMP/bin --config-dir TMP/config --skills-dir TMP/skills；手动启动临时服务并验证在线与构建。
真实部署使用与用户角色匹配的服务管理器；检查status、node show、构建成功与制品内容。重跑安装应保留所有token与配置。
