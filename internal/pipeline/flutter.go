package pipeline

import (
	"context"
	"errors"
	"strings"
	"time"

	"mybuilds/internal/config"
	"mybuilds/internal/mobile"
	"mybuilds/internal/process"
)

// ErrFlutterCleanup保留真实体检的未知清理事实，供Agent阻止零动作停止确认。
var ErrFlutterCleanup = errors.New("flutter: cleanup_error")

// 只检查实际选中的生效步骤环境；不调用仓库脚本，也不授予工具材料秘密。
func (p *runPreparation) flutterTools(definition *config.Build, prepared *preparedBuild, params map[string]string) error {
	if definition.Runner == nil || definition.Runner.Framework != "flutter" {
		return nil
	}
	active := false
	for _, step := range prepared.steps {
		active = active || !step.skipped
	}
	if !active {
		return nil
	}
	checked := map[string]bool{}
	steps := append([]preparedStep{}, prepared.steps...)
	steps = append(steps, prepared.success...)
	steps = append(steps, prepared.failure...)
	steps = append(steps, prepared.always...)
	for _, step := range steps {
		if step.skipped {
			continue
		}
		env := process.HostEnvironment()
		for _, item := range step.command.Env {
			key, value, ok := strings.Cut(item, "=")
			if ok {
				env[key] = value
			}
		}
		// artifact 没有命令环境，仍消费本构建明确声明的工具环境。
		if step.step.Kind == "artifact" {
			facts := make(map[string]string, len(p.facts)+2)
			for key, value := range p.facts {
				facts[key] = value
			}
			facts["build.name"] = prepared.name
			facts["step.name"] = step.step.Name
			for key, value := range definition.Env {
				var err error
				env[key], err = p.envValue(value, params, facts)
				if err != nil {
					return err
				}
			}
		}
		// 每一种实际工具环境独立诊断，步骤名不会改变 SDK 身份。
		var identity strings.Builder
		for _, key := range []string{"PATH", "JAVA_HOME", "ANDROID_HOME", "ANDROID_SDK_ROOT", "FLUTTER_ROOT", "PUB_CACHE"} {
			identity.WriteString(key)
			identity.WriteByte(0)
			identity.WriteString(env[key])
			identity.WriteByte(0)
		}
		if checked[identity.String()] {
			continue
		}
		checked[identity.String()] = true
		ctx := p.ctx
		stopAuthority := func() {}
		if p.remote != nil {
			ctx, stopAuthority = p.remote.merge(ctx)
		}
		maximum := time.Minute
		if prepared.timeout > 0 {
			maximum = min(maximum, prepared.timeout)
		}
		if p.remote != nil {
			remaining, _ := p.remote.budgets()
			if remaining != nil {
				maximum = min(maximum, time.Duration(*remaining))
			}
		}
		if maximum <= 0 {
			stopAuthority()
			return errors.New("flutter: tool_timeout")
		}
		bounded, cancel := context.WithTimeout(ctx, maximum)
		started := time.Now()
		checks := mobile.FlutterDoctor(bounded, mobile.FlutterDoctorOptions{Workspace: p.root, Environment: env, Platforms: []string{definition.Runner.Platform}})
		elapsed := time.Since(started)
		cancelled := bounded.Err()
		cancel()
		stopAuthority()
		for _, check := range checks {
			if check.Reason == "cleanup_error" {
				return ErrFlutterCleanup
			}
		}
		if p.remote == nil && prepared.timeout > 0 {
			prepared.timeout -= elapsed
			if prepared.timeout <= 0 {
				return errors.New("flutter: tool_timeout")
			}
		}
		if cancelled != nil {
			return errors.New("flutter: cancelled")
		}
		for _, check := range checks {
			if check.Status != "passed" {
				return errors.New("flutter: " + check.Reason)
			}
		}
	}
	return nil
}
