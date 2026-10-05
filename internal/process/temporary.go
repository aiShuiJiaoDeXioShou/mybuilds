package process

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// TemporaryDirectory在工作树之外创建本次私有目录；调用方已核验工作区。
func TemporaryDirectory(workspace, prefix string) (string, error) {
	for _, temporary := range []string{os.TempDir(), "/tmp"} {
		base, err := filepath.EvalSymlinks(temporary)
		if err != nil {
			continue
		}
		base, err = filepath.Abs(base)
		if err != nil {
			continue
		}
		relative, err := filepath.Rel(workspace, base)
		if err != nil || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
			continue
		}
		if directory, err := os.MkdirTemp(base, prefix); err == nil {
			return directory, nil
		}
	}
	return "", errors.New("临时目录不可用")
}
