package scm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReadPipelineSourceModesAndParentBoundaries(t *testing.T) {
	o, first := gitFixture(t, "sha1")
	o.Ref = first
	o.FileMode = "optional"
	o.File = "absent.yml"
	snap, err := ReadPipeline(context.Background(), o)
	if err != nil || !snap.Missing || snap.SHA != first || len(snap.Content) != 0 || snap.Digest != "" {
		t.Fatalf("precise missing: %v", err)
	}
	o.FileMode = "none"
	o.File = "mybuilds.yml"
	snap, err = ReadPipeline(context.Background(), o)
	if err != nil || snap.Missing || snap.SHA != first || len(snap.Content) != 0 || snap.Digest != "" {
		t.Fatalf("none: %v", err)
	}
	o.FileMode = "optional"
	if err := os.Mkdir(filepath.Join(o.Repository, "directory"), 0700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, o.Repository, "directory/pipeline.yml", "version: 1\nsteps: []\n")
	if err := os.Symlink("directory", filepath.Join(o.Repository, "linked")); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, o.Repository, "plain", "not a directory")
	fixtureGit(t, o.Repository, "add", "directory", "linked", "plain")
	fixtureGit(t, o.Repository, "update-index", "--add", "--cacheinfo", "160000,"+first+",submodule")
	fixtureGit(t, o.Repository, "commit", "-m", "parents")
	o.Ref = ""
	for _, file := range []string{"linked/pipeline.yml", "plain/pipeline.yml", "submodule/pipeline.yml", "directory"} {
		o.File = file
		_, err := ReadPipeline(context.Background(), o)
		requireCode(t, err, "scm_config_not_regular")
	}
	o.File = "directory/absent.yml"
	snap, err = ReadPipeline(context.Background(), o)
	if err != nil || !snap.Missing {
		t.Fatalf("missing leaf: %v", err)
	}
	o.File = "missing/path/pipeline.yml"
	snap, err = ReadPipeline(context.Background(), o)
	if err != nil || !snap.Missing {
		t.Fatalf("missing parent: %v", err)
	}
	o.FileMode = "invalid"
	_, err = ReadPipeline(context.Background(), o)
	requireCode(t, err, "scm_input_invalid")
	requireClean(t, o.DataDir)
}
