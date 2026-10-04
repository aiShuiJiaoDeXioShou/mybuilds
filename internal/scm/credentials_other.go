//go:build !darwin && !linux

package scm

func readPrivate(string, int64) ([]byte, error) { return nil, failure("platform_unsupported") }
