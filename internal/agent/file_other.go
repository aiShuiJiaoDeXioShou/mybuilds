//go:build !darwin && !linux

package agent

import (
	"context"
	"os"
)

func (*dataLock) atomicFile(string, []byte, os.FileInfo) (os.FileInfo, error) {
	return nil, failure("unsupported")
}
func (*dataLock) removeFile(string, os.FileInfo) error { return failure("unsupported") }

func readSecretFile(string) ([]byte, error)     { return nil, failure("unsupported") }
func (*dataLock) resultParent() (string, error) { return "", failure("unsupported") }

func openSnapshot(context.Context, string, localArtifact) (*os.File, error) {
	return nil, failure("unsupported")
}

func (*dataLock) readJournalFile(string) ([]byte, os.FileInfo, error) {
	return nil, nil, failure("unsupported")
}

func (*dataLock) journalNames() ([]string, error) { return nil, failure("unsupported") }
