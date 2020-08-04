package acceptance

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/acceptance/variables"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

var (
	rootBuilderBinaryDir     = filepath.Join("testdata", "builder", "image", "container", "cnb", "lifecycle")
	rootBuilderDockerContext = filepath.Join("testdata", "builder", "image")
	rootBuilderImage         = "lifecycle/acceptance/builder"
	rootBuilderPath          = "/cnb/lifecycle/builder"
)

func TestRootBuilder(t *testing.T) {
	h.SkipIf(t, runtime.GOOS == "windows", "These tests need to be adapted to work on Windows")
	rand.Seed(time.Now().UTC().UnixNano())

	info, err := h.DockerCli(t).Info(context.TODO())
	h.AssertNil(t, err)
	daemonOS = info.OSType

	// Setup test container

	h.MakeAndCopyLifecycle(t, daemonOS, rootBuilderBinaryDir)
	h.DockerBuild(t,
		rootBuilderImage,
		rootBuilderDockerContext,
		h.WithFlags("-f", filepath.Join(rootBuilderDockerContext, variables.DockerfileName)),
	)
	defer h.DockerImageRemove(t, rootBuilderImage)

	spec.Run(t, "acceptance-builder", testRootBuilder, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testRootBuilder(t *testing.T, when spec.G, it spec.S) {
	when("called", func() {
		it("does something", func() {
			h.SkipIf(t, runtime.GOOS == "windows", "Not relevant on Windows")

			output := h.DockerRun(t,
				rootBuilderImage,
				h.WithBash(fmt.Sprintf("%s -stack-group /cnb/stack-group.toml -plan /cnb/plan.toml; tar tvf /layers/example_stack.tgz", rootBuilderPath)),
			)

			h.AssertMatch(t, output, ".wh.sbin")
			h.AssertMatch(t, output, "bin/exe-to-snapshot")
		})
	})
}
