# 008 终态只读协议

```go
type TerminalReceiptRequest struct {
    Ref LeaseRef `json:"ref"`
    Seq int64 `json:"seq"`
    Digest string `json:"digest"`
}
type TerminalReceipt struct {
    Ref LeaseRef `json:"ref"`
    Seq int64 `json:"seq"`
    Digest string `json:"digest"`
    Status string `json:"status"`
    StopKnown bool `json:"stop_known"`
    NodeName string `json:"node_name"`
}
```

Ref沿007完整六项，Seq正数，Digest64位小写SHA256；返回Ref/Seq/Digest逐项相等，Status为该已确认终态succeeded/failed/cancelled/skipped，StopKnown必须true。NodeName来自当前token实际节点，Agent必须等于cfg.Node。Kind/StopKnown是私有receipt持久字段；Kind不新增到ExecutionProgress，状态是中央实际既有终态，不允许调用者提供。receipt.Kind=build_finished及StopKnown只在原ApplyEvent成功验证完整步骤、最后日志/产物cursor/manifest与真实停止后同事务写，cleanup错误终态不能成为true。


此消息仅服务于当前独立节点核对自有待确认终态，HTTP与Go入口引用此处，不扩通用恢复协议。
