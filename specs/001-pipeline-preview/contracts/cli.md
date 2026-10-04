# CLI 契约

- `mybuilds init [--template <本地文件>]`：当前目录排他创建 mybuilds.yml。目标存在、无效模板或写失败返回 1；成功返回 0。默认写 version:1/steps 下的一个 run，不执行该 run。
- framework/platform 参数可识别但当前未支持且非零，不假造平台模板。template 与 framework/platform 显式混用拒绝。
- `mybuilds run --dry-run [--file mybuilds.yml] [--build name,list | --all] [--param key=value ...] [--step name]`。
- 默认文件相对当前目录；--build/--all 互斥，空项、重复/未知选择拒绝；多 build 必须选择；--step 仅一个 build，按实际名称选步，不规避全配置校验。
- --param 第一个 = 分割，未知/重复/空键拒绝，最终约束检查；本功能 run 不增加 version/channel 快捷项（规划只给 trigger）。
- 预览输出稳定 JSON，三态 condition=ready/skipped/pending，参数值、env 值、run/argv 正文、凭据/Webhook 均不输出。
- --dry-run 不读连接/token配置、不调用 exec/Git/HTTP、不解析 secrets；分支事实缺省为未知，changes 标明手动忽略。
- 不带 --dry-run 返回未实现错误，不执行前置动作；保持 version/help/未知命令行为。
