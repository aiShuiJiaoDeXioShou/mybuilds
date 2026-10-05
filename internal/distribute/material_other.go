//go:build !darwin && !linux

package distribute

import "os"

func openMaterial(string, bool) (*os.File, error) { return nil, errMaterial }
