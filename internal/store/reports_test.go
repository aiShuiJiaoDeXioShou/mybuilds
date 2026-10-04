package store

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"mybuilds/internal/protocol"
)

func reportFixture(t *testing.T, s *Store, required bool) (NodeActor, protocol.LeaseGrant) {
	t.Helper()
	return queuedReports(t, s, required, []string{"results/*.xml"}, false)
}
func reportEvidence(id string) protocol.ReportEvidence {
	h := sha256.Sum256([]byte("results/test.xml"))
	return protocol.ReportEvidence{Revision: 1, Outcome: "pending", Required: true, Counts: protocol.JUnitCounts{Tests: 1}, Diagnostics: []protocol.JUnitDiagnostic{}, Files: []protocol.ReportFile{{Key: hex.EncodeToString(h[:]), Path: "results/test.xml", ArtifactID: id, SourceIndex: 1, SourceStep: "compile", Size: 47, SHA256: hex.EncodeToString(h[:]), Counts: protocol.JUnitCounts{Tests: 1}}}}
}
func reportFinished(t *testing.T, s *Store, a NodeActor, g protocol.LeaseGrant) {
	t.Helper()
	p := stepProgress("intent", "", "ordinary", "compile", 1)
	accept(t, s, a, event(g.Ref, 1, p))
	p.Kind = "started"
	p.Started = true
	accept(t, s, a, event(g.Ref, 2, p))
	p.Kind = "finished"
	p.Status = "succeeded"
	p.StopConfirmed = true
	p.ExitCode = 0
	accept(t, s, a, event(g.Ref, 3, p))
}
func TestReportsCheckedActualDispatch(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, g := reportFixture(t, s, true)
		reportFinished(t, s, a, g)
		evidence := reportEvidence("740b69bb-2b8a-4798-95ae-a7524cf82d85")
		p := stepProgress("reports_checked", "", "ordinary", "compile", 1)
		p.Reports = &evidence
		accept(t, s, a, event(g.Ref, 4, p))
		var row buildRecord
		if err := s.db.First(&row, "id = ?", g.Ref.BuildID).Error; err != nil {
			t.Fatal(err)
		}
		if row.ReportRevision != 1 || row.ReportFinal || row.ReportCheckedIndex != 1 || row.ReportsJSON == "" {
			t.Fatal("checked evidence not persisted")
		}
	})
}
