//go:build darwin

package server

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
	"mybuilds/internal/store"
)

func retentionRenameExclusive(fromRoot, toRoot *os.Root, from, to string) error {
	a, b, err := retentionRenameDirectories(fromRoot, toRoot, from, to)
	if err != nil {
		return err
	}
	defer a.Close()
	defer b.Close()
	err = unix.RenameatxNp(int(a.Fd()), from, int(b.Fd()), to, unix.RENAME_EXCL)
	if errors.Is(err, unix.EEXIST) {
		return store.ErrRetentionOwnershipUnknown
	}
	if err != nil {
		return store.ErrRetentionIO
	}
	return nil
}
