# 003 研究

采用既定doublestar/v4，Go模块版本列表确认当前v4.10.2；锁定版本，不手写**。上游GlobWalk只访问传入fs.FS，Root.FS可限制实际访问；NoFollow不跟随通配目录链接，但首通配前字面目录链接仍跟随，Root限制在根内。FilesOnly不证明普通文件，必须Root.Stat/打开后Stat。每模式独立计数，跨模式去重。来源：[官方源码](https://github.com/bmatcuk/doublestar/blob/v4.10.2/globwalk.go)、[选项](https://github.com/bmatcuk/doublestar/blob/v4.10.2/globoptions.go)、[Go Root](https://pkg.go.dev/os@go1.25.4#Root)。实现前再核对锁定模块源码。

独立临时步骤目录保存files/<source>与manifest.json，全部完成再rename到结果根内对应步骤路径，取消仅删除自己暂存。每个普通/post步骤独立命名空间，后者不能改前者。实际字节数/hash来自复制流，读前后检查源元数据。

复用002流式UTC脱敏记录，同时在Root下按步骤保存；每次Run结果根在预检查后创建，失败保留已完整证据，不存源码。全跳过不创建。日志关闭错误使原成功失败，保留原失败/取消。
