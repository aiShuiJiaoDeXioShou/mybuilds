package pipeline

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoteFactsComeFromTaskAndActualExecutionInsteadOfParameters(t *testing.T) {
	root := t.TempDir()
	document := localDocument(t, `version: 1
params: {payload: default}
env:
 PROJECT: "{{project}}"
 NAME: "{{build.name}}"
 ID: "{{build.id}}"
 NUMBER: "{{build.number}}"
 NODE: "{{node.name}}"
 BRANCH: "{{git.branch}}"
 WORKSPACE: "{{workspace}}"
 STEP: "{{step.name}}"
 VALUE: "${TASK_SECRET}"
steps:
 - kind: run
   name: action
   run: printf '%s|' "$PROJECT" "$NAME" "$ID" "$NUMBER" "$NODE" "$BRANCH" "$WORKSPACE" "$STEP" "$VALUE" > facts
`)
	remote := remoteOptions(t)
	remote.Facts = map[string]string{"project": "trusted-project", "build.id": "trusted-id", "build.number": "42", "node.name": "trusted-node", "git.branch": "main", "build.name": "forged-remote-name", "workspace": "forged-remote-workspace", "step.name": "forged-remote-step"}
	literal := "${NOT_READ} {{params.payload}}"
	remote.Secrets = map[string]string{"TASK_SECRET": literal}
	options := RunOptions{Workspace: root, Remote: remote, PreviewOptions: PreviewOptions{Params: map[string]string{"payload": "forged-param"}, Facts: map[string]string{"project": "forged-preview", "build.id": "forged-id", "node.name": "forged-node", "git.branch": "forged-branch"}}}
	options.Params["project"] = "forged-reserved-param"
	if result, err := runWithCleanup(t, context.Background(), document, options); err == nil || result != nil {
		t.Fatal("reserved fact parameter accepted", err)
	}
	requireAbsent(t, root, "facts")
	delete(options.Params, "project")
	actualRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runWithCleanup(t, context.Background(), document, options); err != nil {
		t.Fatal(err)
	}
	requireFile(t, root, "facts", strings.Join([]string{"trusted-project", "default", "trusted-id", "42", "trusted-node", "main", actualRoot, "action", literal, ""}, "|"))
}
