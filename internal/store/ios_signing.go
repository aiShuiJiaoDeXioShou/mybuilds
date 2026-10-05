package store

import (
	"encoding/json"
)

// 原签名配置来自冻结快照，不能以节点提供的步骤或进程标记代替原生资源关闭。
func requiresIOSCleanup(row buildRecord) (bool, error) {
	var snapshot BuildSnapshot
	if json.Unmarshal([]byte(row.SnapshotJSON), &snapshot) != nil {
		return false, errDatabase
	}
	return snapshot.Definition.IOSSigning != nil, nil
}
