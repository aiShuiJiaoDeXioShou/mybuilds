# 安装入口

Shell：install-client.sh / install-server.sh，支持 --version VERSION（默认v0.1.0），下载校验后转交安装器。包内 install.sh ROLE 可离线执行。
Unix安装器：--bin-dir、--config-dir、--skills-dir；client 支持 --server-url、--token-file、--ca-file；server 支持 --with-agent、--port（默认8787）、--service user/system/none（默认user）。system须以普通用户执行且可用sudo；服务仍以该用户运行。已存在连接配置不覆盖。
Windows客户端：install-client.ps1 的 -Version、-BinDir、-ServerUrl、-TokenFile；服务端 install-server.ps1 的 -Distro、-Version、-WithAgent，转发到已启用systemd的WSL用户环境。
安装器退出0表示安装和所请求服务检查完成；错误非0。不能静默降级服务管理方式。
