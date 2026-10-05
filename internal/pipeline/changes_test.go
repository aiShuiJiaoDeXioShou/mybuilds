package pipeline

import (
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"strings"
	"testing"
)

func TestChangesConditionsKeepManualAndFreezeAND(t *testing.T) {
	when := &config.When{Changes: []string{"android/**", "shared/**"}, Branches: []string{"main"}, Params: map[string]string{"channel": "internal"}}
	target := strings.Repeat("a", 40)
	baseline := strings.Repeat("b", 40)
	cases := []struct {
		name                  string
		facts                 *protocol.ChangeFacts
		branch, channel, want string
	}{
		{"manual", nil, "main", "internal", "ready"},
		{"emptydiff", &protocol.ChangeFacts{Mode: "diff", BaselineSHA: baseline, TargetSHA: target, Paths: []string{}}, "main", "internal", "skipped"},
		{"shared", &protocol.ChangeFacts{Mode: "diff", BaselineSHA: baseline, TargetSHA: target, Paths: []string{"shared/a"}}, "main", "internal", "ready"},
		{"paramAND", &protocol.ChangeFacts{Mode: "full", Reason: "baseline_missing", TargetSHA: target, Paths: []string{}}, "main", "wrong", "skipped"},
		{"branchAND", &protocol.ChangeFacts{Mode: "full", Reason: "baseline_missing", TargetSHA: target, Paths: []string{}}, "other", "internal", "skipped"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if item.facts != nil {
				item.facts.Digest = protocol.ChangesDigest(item.facts.Paths)
			}
			got := WhenCondition(when, map[string]string{"channel": item.channel}, map[string]string{"git.branch": item.branch}, item.facts)
			if got.Condition != item.want {
				t.Fatalf("%s %+v", item.want, got)
			}
		})
	}
}
