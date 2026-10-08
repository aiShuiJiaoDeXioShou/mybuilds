package agent

import (
	"context"
	"maps"
	"mybuilds/internal/config"
	"mybuilds/internal/distribute"
	"mybuilds/internal/process"
	"mybuilds/internal/protocol"
	"mybuilds/internal/scm"
	"strings"
)

// 管理查询只消费原冻结声明及当前明确secret；不调用Run或下载原artifact。
func runCustomPublishQuery(ctx context.Context, cfg config.AgentConfig, task protocol.PublishQueryTask, values map[string]string) (protocol.PublishQueryResult, error) {
	c := task.Custom
	if c == nil || !validRef(c.OriginalRef) || c.OriginalRef.NodeID != task.NodeID || !safeDigest(c.AuthorizationDigest) || !safeDigest(c.ArtifactSHA256) || !exactUUID(c.ArtifactID) || c.Number < 1 || c.Number != task.VersionCode || c.ReportIDs == nil {
		return protocol.PublishQueryResult{}, failure("invalid_response")
	}
	secrets := taskSecrets(cfg, config.Build{Env: c.Environment, Steps: []config.Step{{Kind: "upload", Credentials: task.CredentialRef}}}, values)
	checkout, e := scm.Checkout(ctx, scm.CheckoutOptions{DataDir: cfg.DataDir, Repository: c.Repository, Branch: c.Facts["git.branch"], SHA: c.SHA, SSHKey: values["MYBUILDS_GIT_SSH_KEY"], KnownHosts: values["MYBUILDS_GIT_KNOWN_HOSTS"]})
	if !checkout.StopConfirmed {
		return protocol.PublishQueryResult{}, distribute.ErrCleanup
	}
	if e != nil {
		return protocol.PublishQueryResult{}, e
	}
	facts := maps.Clone(c.Facts)
	if facts == nil {
		facts = map[string]string{}
	}
	facts["workspace"], facts["node.name"] = checkout.Workspace, cfg.Node
	facts["git.sha"], facts["build.id"] = c.SHA, c.OriginalRef.BuildID
	env := process.HostEnvironment()
	for key, value := range c.Environment {
		rendered, e := renderCustomEnvironment(value, c.Params, facts, secrets)
		if e != nil {
			return protocol.PublishQueryResult{}, e
		}
		env[key] = rendered
	}
	working, missing, e := config.RenderField(c.WorkingDir, "step.working_dir", c.Params, facts, false, false)
	if e != nil || missing {
		return protocol.PublishQueryResult{}, failure("publish_precheck")
	}
	result, missing, e := config.RenderField(c.ResultFile, "step.result_file", c.Params, facts, false, false)
	if e != nil || missing {
		return protocol.PublishQueryResult{}, failure("publish_precheck")
	}
	credential := ""
	if task.CredentialRef != "" {
		name := strings.TrimSuffix(strings.TrimPrefix(task.CredentialRef, "${"), "}")
		var ok bool
		credential, ok = secrets[name]
		if !ok {
			return protocol.PublishQueryResult{}, failure("publish_precheck")
		}
	}
	secretValues := []string{}
	for _, v := range secrets {
		if v != "" {
			secretValues = append(secretValues, v)
		}
	}
	return distribute.QueryCustom(ctx, distribute.CustomOptions{Workspace: checkout.Workspace, DataDir: cfg.DataDir, WorkingDir: working, ResultFile: result, QueryArgv: c.QueryArgv, Environment: env, Params: c.Params, Credentials: credential, SecretValues: secretValues, AppIdentifier: task.AppIdentifier, VersionName: c.VersionName, Number: c.Number, ArtifactID: c.ArtifactID, ArtifactSize: c.ArtifactSize, ArtifactSHA256: c.ArtifactSHA256}, task)
}

func renderCustomEnvironment(value string, params, facts, secrets map[string]string) (string, error) {
	var out strings.Builder
	for rest := value; ; {
		start := strings.Index(rest, "${")
		literal := rest
		if start >= 0 {
			literal = rest[:start]
		}
		rendered, missing, e := config.RenderField(literal, "env", params, facts, false, false)
		if e != nil || missing {
			return "", failure("publish_precheck")
		}
		out.WriteString(rendered)
		if start < 0 {
			break
		}
		rest = rest[start+2:]
		end := strings.IndexByte(rest, '}')
		if end < 0 || !environmentName.MatchString(rest[:end]) {
			return "", failure("publish_precheck")
		}
		secret, ok := secrets[rest[:end]]
		if !ok {
			return "", failure("publish_precheck")
		}
		out.WriteString(secret)
		rest = rest[end+1:]
	}
	if strings.ContainsRune(out.String(), 0) {
		return "", failure("publish_precheck")
	}
	return out.String(), nil
}
