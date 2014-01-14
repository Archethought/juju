// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"os"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/storage"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
)

type bootstrapSuite struct {
	testing.JujuConnSuite
	env *localStorageEnviron
}

var _ = gc.Suite(&bootstrapSuite{})

type localStorageEnviron struct {
	environs.Environ
	storage           storage.Storage
	storageAddr       string
	storageDir        string
	sharedStorageAddr string
	sharedStorageDir  string
}

func (e *localStorageEnviron) Storage() storage.Storage {
	return e.storage
}

func (e *localStorageEnviron) StorageAddr() string {
	return e.storageAddr
}

func (e *localStorageEnviron) StorageDir() string {
	return e.storageDir
}

func (e *localStorageEnviron) SharedStorageAddr() string {
	return e.sharedStorageAddr
}

func (e *localStorageEnviron) SharedStorageDir() string {
	return e.sharedStorageDir
}

func (s *bootstrapSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.env = &localStorageEnviron{
		Environ:    s.Conn.Environ,
		storageDir: c.MkDir(),
	}
	storage, err := filestorage.NewFileStorageWriter(s.env.storageDir, filestorage.UseDefaultTmpDir)
	c.Assert(err, gc.IsNil)
	s.env.storage = storage
}

func (s *bootstrapSuite) getArgs(c *gc.C) BootstrapArgs {
	hostname, err := os.Hostname()
	c.Assert(err, gc.IsNil)
	toolsList, err := tools.FindBootstrapTools(s.Conn.Environ, tools.BootstrapToolsParams{})
	c.Assert(err, gc.IsNil)
	arch := "amd64"
	return BootstrapArgs{
		Host:          hostname,
		DataDir:       "/var/lib/juju",
		Environ:       s.env,
		PossibleTools: toolsList,
		Series:        "precise",
		HardwareCharacteristics: &instance.HardwareCharacteristics{
			Arch: &arch,
		},
		Context: envtesting.NewBootstrapContext(coretesting.Context(c)),
	}
}

func (s *bootstrapSuite) TestBootstrap(c *gc.C) {
	args := s.getArgs(c)
	args.Host = "ubuntu@" + args.Host

	defer fakeSSH{SkipDetection: true}.install(c).Restore()
	err := Bootstrap(args)
	c.Assert(err, gc.IsNil)

	bootstrapState, err := bootstrap.LoadState(s.env.Storage())
	c.Assert(err, gc.IsNil)
	c.Assert(
		bootstrapState.StateInstances,
		gc.DeepEquals,
		[]instance.Id{BootstrapInstanceId},
	)

	// Do it all again; this should work, despite the fact that
	// there's a bootstrap state file. Existence for that is
	// checked in general bootstrap code (environs/bootstrap).
	defer fakeSSH{SkipDetection: true}.install(c).Restore()
	err = Bootstrap(args)
	c.Assert(err, gc.IsNil)

	// We *do* check that the machine has no juju* upstart jobs, though.
	defer fakeSSH{
		Provisioned:        true,
		SkipDetection:      true,
		SkipProvisionAgent: true,
	}.install(c).Restore()
	err = Bootstrap(args)
	c.Assert(err, gc.Equals, ErrProvisioned)
}

func (s *bootstrapSuite) TestBootstrapScriptFailure(c *gc.C) {
	args := s.getArgs(c)
	args.Host = "ubuntu@" + args.Host
	defer fakeSSH{SkipDetection: true, ProvisionAgentExitCode: 1}.install(c).Restore()
	err := Bootstrap(args)
	c.Assert(err, gc.NotNil)

	// Since the script failed, the state file should have been
	// removed from storage.
	_, err = bootstrap.LoadState(s.env.Storage())
	c.Check(err, gc.Equals, environs.ErrNotBootstrapped)
}

func (s *bootstrapSuite) TestBootstrapEmptyDataDir(c *gc.C) {
	args := s.getArgs(c)
	args.DataDir = ""
	c.Assert(Bootstrap(args), gc.ErrorMatches, "data-dir argument is empty")
}

func (s *bootstrapSuite) TestBootstrapEmptyHost(c *gc.C) {
	args := s.getArgs(c)
	args.Host = ""
	c.Assert(Bootstrap(args), gc.ErrorMatches, "host argument is empty")
}

func (s *bootstrapSuite) TestBootstrapNilEnviron(c *gc.C) {
	args := s.getArgs(c)
	args.Environ = nil
	c.Assert(Bootstrap(args), gc.ErrorMatches, "environ argument is nil")
}

func (s *bootstrapSuite) TestBootstrapNoMatchingTools(c *gc.C) {
	// Empty tools list.
	args := s.getArgs(c)
	args.PossibleTools = nil
	defer fakeSSH{SkipDetection: true, SkipProvisionAgent: true}.install(c).Restore()
	c.Assert(Bootstrap(args), gc.ErrorMatches, "possible tools is empty")

	// Non-empty list, but none that match the series/arch.
	args = s.getArgs(c)
	args.Series = "edgy"
	defer fakeSSH{SkipDetection: true, SkipProvisionAgent: true}.install(c).Restore()
	c.Assert(Bootstrap(args), gc.ErrorMatches, "no matching tools available")
}
