package pipeline

import (
	"context"
	"errors"
	"mybuilds/internal/mobile"
)

// 本次构建的具体原生资源，在普通run准备前持久化，所有post之后独立关闭。
type iosBuildResources struct {
	options    mobile.IOSSigningOptions
	resources  *mobile.IOSResources
	checkpoint func(mobile.IOSResourceOwnership) error
}

func (state *iosBuildResources) save() error {
	if state.checkpoint == nil || state.resources == nil {
		return nil
	}
	return state.checkpoint(state.resources.Ownership())
}
func (state *iosBuildResources) prepare(ctx context.Context) error {
	if state.resources != nil {
		if state.resources.Ownership().Prepared {
			return nil
		}
		return errors.New("ios_prepare_failed")
	}
	resources, err := mobile.PlanIOSResources(ctx, state.options)
	if err != nil {
		return err
	}
	state.resources = resources
	if err = state.save(); err != nil {
		return err
	}
	if state.checkpoint != nil {
		intent := resources.Ownership()
		intent.Preparing = true
		if err = state.checkpoint(intent); err != nil {
			return err
		}
	}
	err = resources.Prepare(ctx, state.options)
	if e := state.save(); e != nil {
		return errors.Join(err, mobile.ErrIOSCleanup)
	}
	return err
}
