package server

import (
	"encoding/json"
	"mybuilds/internal/config"
	"mybuilds/internal/store"
	"strings"
	"testing"
)

func TestProjectProfileSummaryHidesPrivateValues(t *testing.T) {
	p := store.Project{Settings: config.ProjectSettings{Pipeline: &config.PipelineSettings{Source: "profile", Builds: map[string]config.BuildSettings{"android": {Profile: "company", Params: map[string]string{"version": "private-value", "channel": "private-channel"}}, "ios": {Profile: "company"}}}}}
	view := ProjectSummary(p)
	data, e := json.Marshal(view)
	if e != nil || strings.Contains(string(data), "private-") || len(view.ProfileNames) != 1 || view.ProfileNames[0] != "company" || len(view.ParameterKeys["android"]) != 2 {
		t.Fatal("summary leaks values or lacks binding", e, string(data))
	}
}
