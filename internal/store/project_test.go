package store

import (
	"errors"
	"fmt"
	"gorm.io/gorm"
	"mybuilds/internal/config"
	"strings"
	"testing"
)

func projectInput(name string) ProjectInput {
	return ProjectInput{Name: name, Repository: "https://example.org/trusted/repo.git", AllowedNodes: []string{"linux"}, DefaultNode: "linux"}
}
func TestProjectGroupPolicies(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		for _, operation := range []func() error{func() error { _, e := s.RenameGroup(testContext, localAdmin, "default", "renamed"); return e }, func() error { return s.DeleteGroup(testContext, localAdmin, "default") }} {
			if !errors.Is(operation(), ErrForbidden) {
				t.Fatal("default mutable")
			}
		}
		group, err := s.CreateGroup(testContext, localAdmin, "团队")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.CreateGroup(testContext, localAdmin, "团队"); !errors.Is(err, ErrConflict) {
			t.Fatal("duplicate group")
		}
		p, err := s.CreateProject(testContext, localAdmin, projectInput("app"))
		if err != nil {
			t.Fatal(err)
		}
		if p.GroupName != "default" || p.NextNumber != 1 || p.PolicyVersion != 1 {
			t.Fatalf("defaults: %+v", p)
		}
		moved, err := s.MoveProject(testContext, localAdmin, p.Name, group.Name)
		if err != nil || moved.ID != p.ID || moved.GroupID != group.ID || moved.NextNumber != 1 || moved.PolicyVersion != 2 {
			t.Fatalf("move: %+v %v", moved, err)
		}
		var before, after int64
		s.db.Model(&auditRecord{}).Count(&before)
		same, err := s.MoveProject(testContext, localAdmin, p.Name, group.Name)
		s.db.Model(&auditRecord{}).Count(&after)
		if err != nil || same.PolicyVersion != moved.PolicyVersion || after != before {
			t.Fatal("no-op audited or version changed")
		}
		if err = s.DeleteGroup(testContext, localAdmin, group.Name); !errors.Is(err, ErrConflict) {
			t.Fatalf("nonempty group: %v", err)
		}
		renamed, err := s.RenameGroup(testContext, localAdmin, group.Name, "renamed")
		if err != nil || renamed.ID != group.ID || renamed.Name != "renamed" {
			t.Fatalf("rename: %+v %v", renamed, err)
		}
		p, err = s.GetProject(testContext, p.Name)
		if err != nil || p.GroupName != "renamed" {
			t.Fatal("project group stale")
		}
		list, err := s.ListProjects(testContext, ProjectFilter{Group: "renamed"})
		if err != nil || len(list) != 1 {
			t.Fatal("group filter")
		}
		if _, err = s.MoveProject(testContext, localAdmin, p.Name, "missing"); !errors.Is(err, ErrNotFound) {
			t.Fatal("missing target")
		}
		settings := config.ProjectSettings{Pipeline: &config.PipelineSettings{Source: "repo", File: "ci/build.yml", Builds: map[string]config.BuildSettings{"android": {Params: map[string]string{"version": "secret-value-marker"}}}}}
		p, err = s.SetProjectSettings(testContext, localAdmin, p.Name, settings)
		if err != nil || p.Settings.Pipeline.File != "ci/build.yml" || p.PolicyVersion != 3 {
			t.Fatalf("set: %+v %v", p, err)
		}
		if _, err = s.SetProjectSettings(testContext, localAdmin, p.Name, config.ProjectSettings{}); err != nil {
			t.Fatal(err)
		}
		p, _ = s.GetProject(testContext, p.Name)
		if p.Settings.Pipeline == nil {
			t.Fatal("missing block cleared settings")
		}
		if err = s.DeleteProject(testContext, localAdmin, p.Name); err != nil {
			t.Fatal(err)
		}
		if err = s.DeleteGroup(testContext, localAdmin, "renamed"); err != nil {
			t.Fatal(err)
		}
	})
}
func TestProjectValidationAndFK(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		inputs := []ProjectInput{projectInput(strings.Repeat("a", 65)), projectInput("../secret-marker"), projectInput("duplicate")}
		inputs[2].DefaultNode = "outside"
		missing := projectInput("missing-nodes")
		missing.AllowedNodes = nil
		inputs = append(inputs, missing)
		zero := projectInput("bad-number")
		zero.BuildNumberStart = -1
		inputs = append(inputs, zero)
		password := projectInput("bad-repo")
		password.Repository = "https://user:secret-marker@example.org/repo"
		inputs = append(inputs, password)
		for _, input := range inputs {
			if _, err := s.CreateProject(testContext, localAdmin, input); !errors.Is(err, ErrInvalid) {
				t.Fatalf("invalid accepted: %v", err)
			}
		}
		p, err := s.CreateProject(testContext, localAdmin, projectInput("valid"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.CreateProject(testContext, localAdmin, projectInput("valid")); !errors.Is(err, ErrConflict) {
			t.Fatal("duplicate project")
		}
		if err := s.write(testContext, func(tx *gorm.DB) error {
			return tx.Model(&projectRecord{}).Where("id = ?", p.ID).Update("group_id", "missing-id").Error
		}); !errors.Is(err, ErrConflict) {
			t.Fatalf("FK: %v", err)
		}
		if _, err = s.ListProjects(testContext, ProjectFilter{Page: Page{Limit: 201}}); !errors.Is(err, ErrInvalid) {
			t.Fatal("unbounded page")
		}
	})
}

func TestDeleteGroupCreateProjectRace(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		for i := 0; i < 10; i++ {
			name := fmt.Sprintf("race-%d", i)
			if _, err := s.CreateGroup(testContext, localAdmin, name); err != nil {
				t.Fatal(err)
			}
			start := make(chan struct{})
			created := make(chan error, 1)
			deleted := make(chan error, 1)
			go func() {
				<-start
				input := projectInput(name)
				input.Group = name
				_, err := s.CreateProject(testContext, localAdmin, input)
				created <- err
			}()
			go func() { <-start; deleted <- s.DeleteGroup(testContext, localAdmin, name) }()
			close(start)
			createErr, deleteErr := <-created, <-deleted
			if createErr == nil {
				if !errors.Is(deleteErr, ErrConflict) {
					t.Fatalf("nonempty deleted: %v", deleteErr)
				}
			} else if errors.Is(createErr, ErrNotFound) {
				if deleteErr != nil {
					t.Fatalf("empty delete: %v", deleteErr)
				}
			} else {
				t.Fatalf("race: %v %v", createErr, deleteErr)
			}
		}
		var missing int64
		s.db.Raw("SELECT count(*) FROM projects LEFT JOIN project_groups ON project_groups.id = projects.group_id WHERE project_groups.id IS NULL").Scan(&missing)
		if missing != 0 {
			t.Fatal("orphan project")
		}
	})
}
