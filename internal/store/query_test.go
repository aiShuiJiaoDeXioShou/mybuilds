package store

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestBuildQueriesSafetyAndRestart(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		p, err := s.CreateProject(testContext, localAdmin, projectInput("app"))
		if err != nil {
			t.Fatal(err)
		}
		second, err := s.CreateProject(testContext, localAdmin, projectInput("other"))
		if err != nil {
			t.Fatal(err)
		}
		input := enqueueInput(p, "first")
		duration := int64(time.Minute)
		input.Builds[0].InitialBudgetNS = &duration
		input.Builds[0].Snapshot.Definition.Timeout = "1m"
		first, err := s.Enqueue(testContext, input)
		if err != nil {
			t.Fatal(err)
		}
		later, err := s.Enqueue(testContext, enqueueInput(p, "later"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.Enqueue(testContext, enqueueInput(second, "other")); err != nil {
			t.Fatal(err)
		}
		group, err := s.CreateGroup(testContext, localAdmin, "moved")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.MoveProject(testContext, localAdmin, p.Name, group.Name); err != nil {
			t.Fatal(err)
		}
		list, err := s.ListBuilds(testContext, BuildFilter{Project: p.Name, Group: group.Name, BuildName: "android", Status: "queued", Page: Page{Limit: 1}})
		if err != nil || len(list) != 1 || list[0].ID != later.Builds[0].ID {
			t.Fatalf("stable filter: %v %v", list, err)
		}
		list, err = s.ListBuilds(testContext, BuildFilter{Project: second.Name, Group: group.Name})
		if err != nil || len(list) != 0 {
			t.Fatal("filter not intersection")
		}
		list, err = s.ListBuilds(testContext, BuildFilter{BatchID: first.ID})
		if err != nil || len(list) != 1 {
			t.Fatal("batch filter")
		}
		detail, err := s.GetBuild(testContext, first.Builds[0].ID)
		if err != nil || detail.Group != group.Name || detail.RemainingBudgetNS == nil || *detail.RemainingBudgetNS != duration || len(detail.Steps) != 1 || len(detail.Post) != 1 || detail.Steps[0].Status != "pending" || detail.Post[0].Status != "pending" {
			t.Fatalf("detail: %v %v", detail, err)
		}
		bytes, _ := json.Marshal(detail)
		for _, marker := range []string{"secret-param-marker", "secret-script-marker", "secret-post-marker", "Snapshot", "Facts", "Repository", "DigestJSON"} {
			if strings.Contains(string(bytes), marker) {
				t.Fatalf("public leak: %s", marker)
			}
		}
		if len(detail.ParameterKeys) != 1 || detail.ParameterKeys[0] != "version" {
			t.Fatal("parameter keys lost")
		}
		for _, filter := range []BuildFilter{{Page: Page{Limit: 201}}, {Page: Page{Offset: -1}}, {Status: "succeeded"}} {
			if _, err = s.ListBuilds(testContext, filter); !errors.Is(err, ErrInvalid) {
				t.Fatal("invalid filter")
			}
		}
		if err = s.Close(); err != nil {
			t.Fatal(err)
		}
		s, err = Open(testContext, opt)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		reopened, err := s.GetBuild(testContext, detail.ID)
		if err != nil || reopened.CreatedAt != detail.CreatedAt || *reopened.RemainingBudgetNS != duration || reopened.PostBudgetNS != detail.PostBudgetNS || len(reopened.Post) != 1 {
			t.Fatal("restart changed progress or budget")
		}
	})
}
