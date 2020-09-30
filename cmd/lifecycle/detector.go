package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/env"
	"github.com/buildpacks/lifecycle/priv"
)

type detectCmd struct {
	// flags: inputs
	detectArgs

	// flags: paths to write outputs
	groupPath           string
	privilegedGroupPath string
	planPath            string
	runGroupPath        string
	runPlanPath         string
}

type detectArgs struct {
	// inputs needed when run by creator
	buildpacksDir      string
	appDir             string
	platformDir        string
	orderPath          string
	stackOrderPath     string
	stackBuildpacksDir string
}

func (d *detectCmd) Init() {
	cmd.FlagBuildpacksDir(&d.buildpacksDir)
	cmd.FlagAppDir(&d.appDir)
	cmd.FlagPlatformDir(&d.platformDir)
	cmd.FlagStackBuildpacksDir(&d.stackBuildpacksDir)
	cmd.FlagOrderPath(&d.orderPath)
	cmd.FlagStackOrderPath(&d.stackOrderPath)
	cmd.FlagGroupPath(&d.groupPath)
	cmd.FlagPrivilegedGroupPath(&d.privilegedGroupPath)
	cmd.FlagPlanPath(&d.planPath)
	cmd.FlagRunGroupPath(&d.runGroupPath)
	cmd.FlagRunPlanPath(&d.runPlanPath)
}

func (d *detectCmd) Args(nargs int, args []string) error {
	if nargs != 0 {
		return cmd.FailErrCode(errors.New("received unexpected arguments"), cmd.CodeInvalidArgs, "parse arguments")
	}
	return nil
}

func (d *detectCmd) Privileges() error {
	// detector should never be run with privileges
	if priv.IsPrivileged() {
		return cmd.FailErr(errors.New("refusing to run as root"), "build")
	}
	return nil
}

func (d *detectCmd) Exec() error {
	dr, err := d.detect()
	if err != nil {
		return err
	}
	return d.writeData(dr)
}

func (da detectArgs) mergeOrderWithStackBuildpacks(order lifecycle.BuildpackOrder) (lifecycle.BuildpackOrder, error) {
	if _, err := os.Stat(da.stackOrderPath); err != nil {
		if os.IsNotExist(err) {
			return order, nil
		}
	}

	stackBuildPacksOrder, err := lifecycle.ReadOrder(da.stackOrderPath)
	if err != nil {
		return nil, err
	}

	if len(stackBuildPacksOrder) == 0 {
		return order, nil
	}

	if len(stackBuildPacksOrder) > 1 {
		return nil, cmd.FailErr(err, fmt.Sprintf("too many groups defined in stack buildpack order file '%s'", da.stackOrderPath))
	}

	stackBuildpacks := stackBuildPacksOrder[0].Group

	fo := lifecycle.BuildpackOrder{}
	for _, grp := range order {
		fo = append(fo, lifecycle.BuildpackGroup{Group: append(stackBuildpacks, grp.Group...)})
	}

	return fo, nil
}

func (da detectArgs) detect() (lifecycle.DetectResult, error) {
	order, err := lifecycle.ReadOrder(da.orderPath)
	if err != nil {
		return lifecycle.DetectResult{}, cmd.FailErr(err, "read buildpack order file")
	}

	if err := da.validateBuildpacks(order); err != nil {
		return lifecycle.DetectResult{}, cmd.FailErr(err, "validate buildpack")
	}

	order, err = da.mergeOrderWithStackBuildpacks(order)
	if err != nil {
		return lifecycle.DetectResult{}, cmd.FailErr(err, "merge stack buildpacks into order")
	}

	if err := da.verifyBuildpackApis(order); err != nil {
		return lifecycle.DetectResult{}, err
	}

	envv := env.NewBuildEnv(os.Environ())
	fullEnv, err := envv.WithPlatform(da.platformDir)
	if err != nil {
		return lifecycle.DetectResult{}, cmd.FailErr(err, "read full env")
	}
	dr, err := order.Detect(&lifecycle.DetectConfig{
		FullEnv:            fullEnv,
		ClearEnv:           envv.List(),
		AppDir:             da.appDir,
		PlatformDir:        da.platformDir,
		BuildpacksDir:      da.buildpacksDir,
		Logger:             cmd.DefaultLogger,
		StackBuildpacksDir: da.stackBuildpacksDir,
	})
	if err != nil {
		switch err := err.(type) {
		case *lifecycle.Error:
			switch err.Type {
			case lifecycle.ErrTypeFailedDetection:
				cmd.DefaultLogger.Error("No buildpack groups passed detection.")
				cmd.DefaultLogger.Error("Please check that you are running against the correct path.")
				return lifecycle.DetectResult{}, cmd.FailErrCode(err, cmd.CodeFailedDetect, "detect")
			case lifecycle.ErrTypeBuildpack:
				cmd.DefaultLogger.Error("No buildpack groups passed detection.")
				return lifecycle.DetectResult{}, cmd.FailErrCode(err, cmd.CodeFailedDetectWithErrors, "detect")
			default:
				return lifecycle.DetectResult{}, cmd.FailErrCode(err, cmd.CodeDetectError, "detect")
			}
		default:
			return lifecycle.DetectResult{}, cmd.FailErrCode(err, cmd.CodeDetectError, "detect")
		}
	}

	return *dr, nil
}

func (da detectArgs) verifyBuildpackApis(order lifecycle.BuildpackOrder) error {
	for _, group := range order {
		for _, bp := range group.Group {
			bpDir := da.buildpacksDir
			if bp.Privileged {
				bpDir = da.stackBuildpacksDir
			}
			bpTOML, err := bp.Lookup(bpDir)
			if err != nil {
				return cmd.FailErr(err, fmt.Sprintf("lookup buildpack.toml for buildpack '%s'", bp.String()))
			}
			if err := cmd.VerifyBuildpackAPI(bp.String(), bpTOML.API); err != nil {
				return err
			}
		}
	}
	return nil
}

func (da detectArgs) validateBuildpacks(order lifecycle.BuildpackOrder) error {
	for _, group := range order {
		for _, bp := range group.Group {
			if bp.Privileged {
				return fmt.Errorf("%s can not be marked as privileged", bp.String())
			}
		}
	}
	return nil
}

func (d *detectCmd) writeData(dr lifecycle.DetectResult) error {
	if err := lifecycle.WriteTOML(d.groupPath, dr.BuildGroup); err != nil {
		return cmd.FailErr(err, "write buildpack group")
	}

	if len(dr.BuildPrivilegedGroup.Group) > 0 {
		if err := lifecycle.WriteTOML(d.privilegedGroupPath, dr.BuildPrivilegedGroup); err != nil {
			return cmd.FailErr(err, "write privileged buildpack group")
		}
	}

	if err := lifecycle.WriteTOML(d.planPath, dr.BuildPlan); err != nil {
		return cmd.FailErr(err, "write detect plan")
	}

	if len(dr.RunGroup.Group) > 0 {
		if err := lifecycle.WriteTOML(d.runGroupPath, dr.RunGroup); err != nil {
			return cmd.FailErr(err, "write run buildpack group")
		}
	}

	if len(dr.RunPlan.Entries) != 0 {
		if err := lifecycle.WriteTOML(d.runPlanPath, dr.RunPlan); err != nil {
			return cmd.FailErr(err, "write run plan")
		}
	}

	return nil
}
