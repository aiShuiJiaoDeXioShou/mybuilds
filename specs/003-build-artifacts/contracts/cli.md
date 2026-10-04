# 003 CLI

run参数与stdout JSON/stderr日志约定保留，不新增flag。成功预检查且有生效步骤后生成结果临时目录，JSON result_dir给出绝对位置；steps/post的artifacts给出源路径/快照相对路径/大小/hash，log_path给出相对日志路径。用户以result_dir拼接相对路径读取结果。
--step可以独立artifact，准备源文件由用户负责；无匹配失败，不自动使用别次执行结果。post副本与普通副本独立。预览无文件副作用，未交付能力继续明确报错。
