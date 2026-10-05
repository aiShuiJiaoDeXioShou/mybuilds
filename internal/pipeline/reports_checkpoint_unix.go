//go:build darwin || linux

package pipeline

import (
	"mybuilds/internal/protocol"
	"syscall"
)

func savedReportFingerprint(f reportFingerprint) (protocol.ReportFingerprint, error) {
	if f.saved != nil {
		return *f.saved, nil
	}
	if f.info == nil {
		return protocol.ReportFingerprint{}, errReportSave
	}
	stat, ok := f.info.Sys().(*syscall.Stat_t)
	if !ok {
		return protocol.ReportFingerprint{}, errReportSave
	}
	return protocol.ReportFingerprint{Device: uint64(stat.Dev), Inode: uint64(stat.Ino), Mode: uint32(f.info.Mode()), Size: f.info.Size(), ModifiedNS: f.info.ModTime().UnixNano(), SHA256: f.hash}, nil
}
