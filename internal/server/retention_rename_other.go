//go:build !darwin && !linux

package server

import (
	"mybuilds/internal/store"
	"os"
)

func retentionRenameExclusive(fromRoot, toRoot *os.Root, from, to string) error {
	return store.ErrRetentionIO
}
