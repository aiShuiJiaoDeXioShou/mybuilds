package store

import (
	"encoding/json"
	"mybuilds/internal/config"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestOriginFrozenProfileRetryAndConcurrentReplay(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		p, e := s.CreateProject(testContext, localAdmin, projectInput("origin"))
		if e != nil {
			t.Fatal(e)
		}
		in := enqueueInput(p, "source-key")
		in.Source = "profile"
		definition := in.Builds[0].Snapshot.Definition
		in.Builds[0].Snapshot.Origin = &PipelineOrigin{Mode: "profile", Kind: "profile", SHA: in.SHA, Profile: "company", ContentDigest: strings.Repeat("d", 64), DefinitionDigest: DefinitionDigest(definition)}
		type result struct {
			value BatchResult
			err   error
		}
		out := make(chan result, 20)
		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); v, e := s.Enqueue(testContext, in); out <- result{v, e} }()
		}
		wg.Wait()
		close(out)
		var original string
		fresh := 0
		for item := range out {
			if item.err != nil {
				t.Fatal(item.err)
			}
			if !item.value.Replayed {
				fresh++
			}
			if original != "" && original != item.value.Builds[0].ID {
				t.Fatal("new batch on replay")
			}
			original = item.value.Builds[0].ID
			if item.value.Builds[0].Origin == nil || !reflect.DeepEqual(item.value.Builds[0].Origin, in.Builds[0].Snapshot.Origin) {
				t.Fatal("lost frozen origin")
			}
		}
		if fresh != 1 {
			t.Fatal("fresh count", fresh)
		}
		if _, e = s.Cancel(testContext, localAdmin, original); e != nil {
			t.Fatal(e)
		}
		if _, e = s.SetProjectSettings(testContext, localAdmin, p.Name, config.ProjectSettings{Pipeline: &config.PipelineSettings{Source: "repo", Params: map[string]string{"version": "changed-current"}}}); e != nil {
			t.Fatal(e)
		}
		next, e := s.Retry(testContext, localAdmin, RetryInput{BuildID: original, Key: "retry-original-source"})
		if e != nil {
			t.Fatal(e)
		}
		if *next.Builds[0].Number != 2 || next.Builds[0].SHA != in.SHA || !reflect.DeepEqual(next.Builds[0].Origin, in.Builds[0].Snapshot.Origin) {
			t.Fatal("retry re-expanded current source")
		}
		var before, after buildRecord
		s.db.First(&before, "id = ?", original)
		s.db.First(&after, "id = ?", next.Builds[0].ID)
		var a, b BuildSnapshot
		json.Unmarshal([]byte(before.SnapshotJSON), &a)
		json.Unmarshal([]byte(after.SnapshotJSON), &b)
		if !reflect.DeepEqual(a.Definition, b.Definition) || !reflect.DeepEqual(a.Params, b.Params) || !reflect.DeepEqual(a.Origin, b.Origin) || b.Facts["build.id"] == a.Facts["build.id"] {
			t.Fatal("retry snapshot drift")
		}
	})
}

func TestOriginInvalidEvidenceAllocatesNothing(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		p, e := s.CreateProject(testContext, localAdmin, projectInput("origin-invalid"))
		if e != nil {
			t.Fatal(e)
		}
		in := enqueueInput(p, "bad-origin")
		in.Builds[0].Snapshot.Origin = &PipelineOrigin{Mode: "auto", Kind: "profile", SHA: in.SHA, Profile: "company", ContentDigest: strings.Repeat("d", 64), DefinitionDigest: strings.Repeat("e", 64)}
		if _, e = s.Enqueue(testContext, in); e == nil {
			t.Fatal("forged definition digest")
		}
		after, e := s.GetProject(testContext, p.Name)
		if e != nil || after.NextNumber != 1 {
			t.Fatal("bad origin consumed number", e)
		}
		if !validateOrigin(nil, in.Builds[0].Snapshot.Definition) {
			t.Fatal("legacy nil origin inferred or rejected")
		}
	})
}
