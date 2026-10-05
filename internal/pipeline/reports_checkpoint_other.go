//go:build !darwin && !linux

package pipeline

import "mybuilds/internal/protocol"

func savedReportFingerprint(f reportFingerprint) (protocol.ReportFingerprint, error) {
	return protocol.ReportFingerprint{}, errReportSave
}
