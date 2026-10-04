//go:build !darwin && !linux

package store

import (
	"context"
	"os"
)

func supportedPlatform() bool                { return false }
func singleLink(os.FileInfo) bool            { return false }
func (s *Store) openSQLiteLock(string) error { return ErrInvalid }
func releaseFileLock(*os.File) error         { return ErrInvalid }

func readPostgresFile(context.Context, string, bool) ([]byte, error) { return nil, ErrInvalid }
