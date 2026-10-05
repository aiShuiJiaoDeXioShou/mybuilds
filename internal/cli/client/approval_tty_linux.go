//go:build linux

package client

import "golang.org/x/sys/unix"

const approvalTermiosRequest = unix.TCGETS
