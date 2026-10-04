package scm

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// sshEnvironment只从明确受限文件取两条材料路径；其他秘密不注入Git。
func sshEnvironment(workspace, secretsFile string) (map[string]string, error) {
	if secretsFile == "" {
		return nil, failure("credentials_invalid")
	}
	filename, err := filepath.Abs(secretsFile)
	if err != nil {
		return nil, failure("credentials_invalid")
	}
	data, err := readPrivate(filename, 64<<10)
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, failure("credentials_invalid")
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "GIT_SSH_KEY_FILE" && key != "GIT_SSH_KNOWN_HOSTS_FILE" {
			if strings.HasPrefix(key, "GIT_") {
				return nil, failure("credentials_invalid")
			}
			continue
		}
		if _, exists := values[key]; exists || value == "" || controls(value) {
			return nil, failure("credentials_invalid")
		}
		values[key] = value
	}
	if len(values) != 2 {
		return nil, failure("credentials_invalid")
	}
	copies := map[string]string{}
	for key, value := range values {
		if !filepath.IsAbs(value) {
			value = filepath.Join(filepath.Dir(filename), value)
		}
		content, err := readPrivate(value, 1<<20)
		if err != nil {
			return nil, err
		}
		destination := filepath.Join(workspace, key)
		file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return nil, failure("credentials_invalid")
		}
		_, writeErr := file.Write(content)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			return nil, failure("credentials_invalid")
		}
		copies[key] = destination
	}
	ssh, err := exec.LookPath("ssh")
	if err != nil {
		return nil, failure("credentials_invalid")
	}
	ssh, err = filepath.Abs(ssh)
	if err != nil {
		return nil, failure("credentials_invalid")
	}
	args := []string{quoteSSH(ssh), "-F /dev/null", "-o BatchMode=yes", "-o IdentitiesOnly=yes", "-o IdentityAgent=none", "-o AddKeysToAgent=no", "-o ForwardAgent=no", "-o StrictHostKeyChecking=yes", "-o GlobalKnownHostsFile=/dev/null", "-o PasswordAuthentication=no", "-o KbdInteractiveAuthentication=no", "-o PreferredAuthentications=publickey", "-o ControlMaster=no", "-o ControlPath=none", "-o ProxyCommand=none", "-o ProxyJump=none", "-o PermitLocalCommand=no", "-o ConnectionAttempts=1", "-o ConnectTimeout=5", "-o LogLevel=ERROR", "-o " + quoteSSH("UserKnownHostsFile="+copies["GIT_SSH_KNOWN_HOSTS_FILE"]), "-i " + quoteSSH(copies["GIT_SSH_KEY_FILE"])}
	return map[string]string{"GIT_SSH_COMMAND": strings.Join(args, " "), "GIT_SSH_VARIANT": "ssh"}, nil
}
func quoteSSH(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
