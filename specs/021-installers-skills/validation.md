# 验证记录

2026-10-08：specify → plan → tasks → analyze → implement 已完成；已有constitution 2.1.0符合范围，无需修改。扩展hooks为空。

## 静态与行为验证

- 9项功能要求均有任务覆盖；无原则冲突或阻塞歧义。
- 6个平台/架构编译通过：macOS、Linux、Windows，各amd64/arm64；macOS启用cgo。
- macOS arm64真实发行包隔离验收通过：初始化与独立token、0600配置、空格路径、重复安装、真实控制端/Agent在线、在线重跑不重置、客户端导入、不覆盖配置/程序、宽权限token拒绝、损坏下载拒绝。
- 两个skill通过 skill-creator quick_validate。
- Shell语法、Python编译、文档链接与git diff --check通过。
- 独立终审发现的Windows并发写入/身份覆盖和Unix环境变量污染已修正。

## 发布与部署验收

- 公开仓库已建立，安装功能已合并main并推送；版本标签v0.1.0指向7abe617。
- GitHub Actions [37660111125](https://github.com/aiShuiJiaoDeXioShou/mybuilds/actions/runs/37660111125) 全部成功：Windows PowerShell真实客户端首次/重复安装及保护检查；Linux和macOS真实控制端/Agent隔离验收。
- 用户指定服务器为macOS arm64：安装控制端和独立local Agent，系统LaunchDaemon以普通服务用户运行，回环监听；实际status与节点session_active/healthy验证通过。
- 本地客户端和两个skills已安装，zsh新会话PATH生效；专用受限SSH密钥与用户LaunchAgent隧道连接控制端。
- installation-demo构建6cbd613e-9d12-4050-9945-e1b8f442b7dc成功，本地下载hello.txt并确认内容及CLI的大小/SHA256校验。未安装或声称验收缺失的移动SDK。
- [v0.1.0](https://github.com/aiShuiJiaoDeXioShou/mybuilds/releases/tag/v0.1.0) 已公开发布，六个平台包及SHA256SUMS均上传成功。使用公开tag下install-client.sh再次真实下载、校验并安装，原有客户端连接保持不变。

## 平台验证边界

Windows原生客户端仍不支持artifact download，已在脚本、README、CLI文档与操作skill明确；服务端依赖用户预先配置的WSL2/systemd。WSL封装脚本已通过PowerShell语法检查，本次没有真实WSL服务启动验收；Linux安装器已在真实Linux runner验收。Linux systemd定义与macOS用户服务定义未在runner上启动，本次生产部署实际验证了macOS系统LaunchDaemon。


## 最终收敛

converge核对9项功能要求、4项成功指标、3个用户故事与5项项目原则，未发现需追加的实现任务，tasks保持全完成状态。未修改Go业务实现或数据库模型；本次没有新依赖。
main分支的三平台验收 [37660471989](https://github.com/aiShuiJiaoDeXioShou/mybuilds/actions/runs/37660471989) 同样全部成功。服务端安装采用已校验的离线包；客户端公开下载入口随后验证通过。运行凭据、SSH密钥与部署地址均未写入项目文件。

## 2026-10-09 本机客户端升级与Java工作流

经用户授权，main的21116f7、2d18771已推送origin/main；未创建发行标签或GitHub Release。按现有release.py从2d18771构建darwin/arm64的v0.1.1-dev.2d18771包（三入口启用cgo），本次只替换本机客户端。包SHA256 b9f609309397aef5c620cbc75e5a914bef86c6be1cf07ba570e0fb59f0114b81，安装后客户端SHA256 3208830e28af3c095a72a793eccd3cf971e62fbbf984836ab0bdebd04d1159e1；程序version和help通过，安装器版本记录同步。旧程序、安装记录、连接配置及工作流备份于~/.mybuilds/updates/20261009-2d18771/backup，连接配置字节不变；没有重新创建身份或项目。

更新~/.mybuilds/build-inputs/sqcms-backend/mybuilds.yml：reports.junit.max_files=1024，路径改target/mybuilds-junit/TEST-*.xml；沿用此前Maven报告兼容化脚本，按输入文件逐份生成，不再合并为一份；原Java路径、Maven退出状态及JAR路径保持。新文件SHA256 96361ad65cffcc9f016922f5501781f977c22e90267b41defedf8979c86012f8。历史source.json记录未改，不把新本地文件冒充旧远端Git快照。

实际新客户端对该文件dry-run exit0。使用该文件中提取的真实转换脚本生成70份Maven格式带命名空间的测试报告，经实际mybuilds run校验：373 tests/16 skipped、70份文件通过；失败用例保留70份且report_failed、统计不一致导致命令失败、max_files=64时70份超限；三种失败均阻止后续哨兵。检查脚本与JSON记录在~/.mybuilds/updates/20261009-2d18771，不含token。生成测试不等于重新运行实际Java项目。

升级后现有SSH隧道status/项目查询正常，远端仍v0.1.0、两项目、一个健康在线节点、无排队/运行构建。本次未升级远端控制端/Agent，未修改其Git仓库里的旧工作流，未触发远端构建；新文件用于远程执行前必须先升级两者并更新实际仓库配置。FRP接入已在023完成specify/plan/tasks/analyze，公网实施待执行。未改Go源，不重复全套Go测试。
