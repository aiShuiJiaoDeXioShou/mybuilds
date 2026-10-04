# 分区公共 API

主代理持有 types.go。A/B/C 不自行调整公共类型，变化先发给主代理。

## config（分区 A）

```go
func Parse(data []byte) (*Document, error)
func Load(filename string) (*Document, error)
func Validate(document *Document) error
func (document *Document) Select(names []string, all bool) ([]string, error)
func ResolveParams(build *Build, overrides map[string]string) (map[string]string, error)
```
Parse/Load 返回规范化的完整集合，所有结构/参数默认/条件参数/路径/专属字段已经检查；不解析 env 引用或模板运行值。
Select 全选字典序稳定，输入列表保持用户顺序；Validate 可验证构造的模型。错误不得包含原标量/原 YAML。

## pipeline（分区 B）

```go
type PreviewOptions struct {
    Names []string
    All bool
    Params map[string]string
    Step string
    Facts map[string]string
}
func Preview(document *config.Document, options PreviewOptions) (*PreviewPlan, error)
```
Facts 使用 git.branch/git.sha/project/build.number/build.id/node.name/workspace 等实际已知上下文；没有事实时标 pending，不伪造。
Preview 先完整校验所选 build 模板与参数，再生成纯数据；普通参数值/环境映射不显示，只展示参数名与隐藏标记。
已知参数匹配 AND，分支 glob OR，不同字段 AND；已知失败覆盖未知；changes 手动忽略且明确说明。
所有字段模板只识别 `{{...}}` 与 env/credentials/notification 中 `${NAME}` 的语法；不解引用密钥，不处理 run/argv 正文。
通知专用模板允许 build.status/build.url；未知/格式错误模板报字段路径，不回显正文；共享通知在各所选 build 的参数上下文校验。
不调用运行器，不新增空接口。PreviewPlan 的具体输出类型由 B 在其文件定义，保证 JSON 可序列化且不会带原配置。

## client（分区 C）
使用上述 Load/Select/Preview API。init 本地模板使用 config.Parse 校验后写原字节；参数错误先于任何写入；排他创建原目标不跟随符号链接。
