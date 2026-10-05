package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mybuilds/internal/config"
	"mybuilds/internal/mobile"
	"mybuilds/internal/scm"
	"mybuilds/internal/store"
)

type resolvedPipeline struct {
	Mode, SHA, SourceDigest, File string
	Document                      *config.Document
	Origins                       map[string]*store.PipelineOrigin
}

func builtinProfiles() map[string][]byte {
	android, _ := mobile.FlutterTemplate([]string{"android"})
	ios, _ := mobile.FlutterTemplate([]string{"ios"})
	return map[string][]byte{"native-android": mobile.AndroidTemplate(), "native-ios": mobile.IOSTemplate(), "flutter-android": android, "flutter-ios": ios}
}

// resolvePipeline只有这一条固定SHA来源链；明确缺失才回退，绝不拼两份集合。
func (s *Server) resolvePipeline(ctx context.Context, p store.Project, branch, ref string) (resolvedPipeline, error) {
	summary := ProjectSummary(p)
	mode := summary.PipelineSource
	if s.profileError != nil {
		return resolvedPipeline{}, errPipeline
	}
	fileMode := "optional"
	if mode == "profile" {
		fileMode = "none"
	} else if mode == "repo" {
		fileMode = "required"
	}
	source, err := scm.ReadPipeline(ctx, scm.Options{DataDir: s.config.DataDir, Repository: p.Repository, Branch: branch, Ref: ref, File: summary.PipelineFile, SecretsFile: s.config.SecretsFile, FileMode: fileMode})
	if err != nil {
		return resolvedPipeline{}, errPipeline
	}
	out := resolvedPipeline{Mode: mode, SHA: source.SHA, File: source.File, SourceDigest: source.Digest, Origins: map[string]*store.PipelineOrigin{}}
	if mode != "profile" && !source.Missing {
		out.Document, err = config.Parse(source.Content)
		if err != nil {
			return resolvedPipeline{}, errPipeline
		}
		for name, b := range out.Document.Builds {
			definition := *b
			definition.Notifications = nil
			out.Origins[name] = &store.PipelineOrigin{Mode: mode, Kind: "repo", SHA: source.SHA, File: source.File, ContentDigest: source.Digest, DefinitionDigest: store.DefinitionDigest(definition)}
		}
		return out, nil
	}
	psettings := p.Settings.Pipeline
	if psettings == nil {
		return resolvedPipeline{}, errPipeline
	}
	bindings := psettings.Builds
	if psettings.Profile != "" {
		bindings = map[string]config.BuildSettings{"default": {Profile: psettings.Profile, Params: psettings.Params}}
	}
	if len(bindings) == 0 {
		return resolvedPipeline{}, errPipeline
	}
	out.Document = &config.Document{Version: 1, Builds: map[string]*config.Build{}}
	for name, binding := range bindings {
		if binding.Profile == "" {
			return resolvedPipeline{}, errPipeline
		}
		profile, ok := s.profiles[binding.Profile]
		if !ok {
			return resolvedPipeline{}, errPipeline
		}
		profile, err = config.CopyLoadedProfile(profile)
		if err != nil {
			return resolvedPipeline{}, errPipeline
		}
		b := profile.Definition
		if b.Notifications == nil {
			b.Notifications = profile.Notifications
		}
		out.Document.Builds[name] = &b
		definition := b
		definition.Notifications = nil
		out.Origins[name] = &store.PipelineOrigin{Mode: mode, Kind: "profile", SHA: source.SHA, Profile: profile.Name, Template: profile.Template, ContentDigest: profile.ContentDigest, DefinitionDigest: store.DefinitionDigest(definition)}
	}
	if config.Validate(out.Document) != nil {
		return resolvedPipeline{}, errPipeline
	}
	data, err := json.Marshal(out.Origins)
	if err != nil {
		return resolvedPipeline{}, errPipeline
	}
	sum := sha256.Sum256(data)
	out.SourceDigest = hex.EncodeToString(sum[:])
	return out, nil
}
