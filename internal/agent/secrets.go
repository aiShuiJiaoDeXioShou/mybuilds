package agent

import (
	"mybuilds/internal/config"
	"regexp"
	"strings"
)

var environmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var secretReference = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func loadSecrets(cfg config.AgentConfig) (map[string]string, error) {
	values := map[string]string{}
	if cfg.SecretsFile == "" {
		return values, nil
	}
	data, err := readSecretFile(cfg.SecretsFile)
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || !environmentName.MatchString(key) || len(value) > 64<<10 || strings.ContainsAny(value, "\x00\r") || len(values) >= 256 {
			return nil, failure("secrets_invalid")
		}
		if _, exists := values[key]; exists {
			return nil, failure("secrets_invalid")
		}
		values[key] = value
	}
	return values, nil
}
func taskSecrets(cfg config.AgentConfig, build config.Build, values map[string]string) (map[string]string, error) {
	secrets := map[string]string{}
	take := func(env map[string]string) error {
		for _, value := range env {
			for _, m := range secretReference.FindAllStringSubmatch(value, -1) {
				name := m[1]
				if name == cfg.TokenEnv || name == "MYBUILDS_AGENT_TOKEN" || name == "MYBUILDS_CLIENT_TOKEN" || strings.HasPrefix(name, "MYBUILDS_BOOTSTRAP_") || strings.HasPrefix(name, "MYBUILDS_SERVER_") || strings.HasPrefix(name, "MYBUILDS_CONTROL_") || name == "MYBUILDS_GIT_SSH_KEY" || name == "MYBUILDS_GIT_KNOWN_HOSTS" {
					continue
				}
				secret, ok := values[name]
				if !ok || secret == cfg.RuntimeToken {
					continue
				}
				secrets[name] = secret
			}
		}
		return nil
	}
	if err := take(build.Env); err != nil {
		return nil, err
	}
	if build.IOSSigning != nil {
		if err := take(map[string]string{"p12": build.IOSSigning.P12, "profile": build.IOSSigning.Profile, "password": build.IOSSigning.Password}); err != nil {
			return nil, err
		}
	}
	steps := append([]config.Step{}, build.Steps...)
	if build.Post != nil {
		steps = append(steps, build.Post.Success...)
		steps = append(steps, build.Post.Failure...)
		steps = append(steps, build.Post.Always...)
	}
	for _, step := range steps {
		if step.Kind == "upload" && step.Credentials != "" {
			if err := take(map[string]string{"credentials": step.Credentials}); err != nil {
				return nil, err
			}
		}
		if err := take(step.Env); err != nil {
			return nil, err
		}
	}
	return secrets, nil
}
