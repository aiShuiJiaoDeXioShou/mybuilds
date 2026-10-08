package store

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
)

func TestReportEvidenceExactCollectionLimits(t *testing.T) {
	makeEvidence := func(n int, size int64) protocol.ReportEvidence {
		e := protocol.ReportEvidence{Revision: 1, Outcome: "pending", Required: true, Files: []protocol.ReportFile{}, Diagnostics: []protocol.JUnitDiagnostic{}}
		for i := 0; i < n; i++ {
			name := fmt.Sprintf("results/%04d.xml", i)
			e.Files = append(e.Files, protocol.ReportFile{Key: reportKey(name), Path: name, ArtifactID: uuid.NewString(), SourceIndex: 1, SourceStep: "compile", Size: size, SHA256: strings.Repeat("a", 64), Counts: protocol.JUnitCounts{}})
		}
		return e
	}
	for _, f := range []struct {
		n     int
		size  int64
		valid bool
	}{{256, 1, true}, {257, 1, false}, {8, 8 << 20, true}, {9, 8 << 20, false}, {1, (8 << 20) + 1, false}} {
		e := makeEvidence(f.n, f.size)
		if validReportEvidence(&e, true, config.DefaultJUnitMaxFiles) != f.valid {
			t.Fatalf("collection limit %d/%d", f.n, f.size)
		}
	}
	e := makeEvidence(1, 1)
	large := makeEvidence(1024, 1)
	if !validReportEvidence(&large, true, 1024) || validReportEvidence(&large, true, 1023) {
		t.Fatal("自定义最大数量未执行")
	}
	e.Files[0].Counts.Tests = 100000
	e.Counts.Tests = 100000
	if !validReportEvidence(&e, true, config.DefaultJUnitMaxFiles) {
		t.Fatal("exact case limit")
	}
	e.Files[0].Counts.Tests++
	e.Counts.Tests++
	if validReportEvidence(&e, true, config.DefaultJUnitMaxFiles) {
		t.Fatal("case overflow")
	}
	e = makeEvidence(1, 1)
	e.Files[0].Counts.DurationNS = int64(365*24*60*60) * 1e9
	e.Counts = e.Files[0].Counts
	if !validReportEvidence(&e, true, config.DefaultJUnitMaxFiles) {
		t.Fatal("exact duration limit")
	}
	e.Files[0].Counts.DurationNS++
	e.Counts = e.Files[0].Counts
	if validReportEvidence(&e, true, config.DefaultJUnitMaxFiles) {
		t.Fatal("duration overflow")
	}
}
