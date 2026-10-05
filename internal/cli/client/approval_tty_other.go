//go:build !darwin && !linux

package client

import (
	"context"
	"errors"
	"io"
	"mybuilds/internal/pipeline"
)

func confirmLocalApproval(io.Reader, io.Writer) func(context.Context, pipeline.ApprovalPrompt) (bool, error) {
	return func(context.Context, pipeline.ApprovalPrompt) (bool, error) {
		return false, errors.New("approval_requires_tty")
	}
}
