// Copyright (C) 2024 Storj Labs, Inc.
// See LICENSE for copying information

package supervisor

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/zeebo/errs"

	"storj.io/common/storj"
	"storj.io/common/version"
)

var errProcess = errs.Class("process")

var errProcessAlreadyStarted = errors.New("process already started")

type Process struct {
	cmd *exec.Cmd

	binPath  string
	storeDir string
	args     []string

	nodeID storj.NodeID

	mu            sync.Mutex
	lastRestarted time.Time
}

func NewProcess(nodeID storj.NodeID, binPath, storeDir string, args []string) *Process {
	return &Process{
		nodeID:   nodeID,
		binPath:  binPath,
		storeDir: storeDir,
		args:     args,
	}
}

// pid returns the process ID of the managed process.
func (p *Process) pid() int {
	if p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

// start starts the process.
// It returns errProcessAlreadyStarted if the process is already started.
func (p *Process) start(ctx context.Context) (err error) {
	if p.cmd != nil {
		return errProcessAlreadyStarted
	}

	p.cmd = exec.CommandContext(ctx, p.args[0], p.args[1:]...)
	p.cmd.Stdout = os.Stdout
	p.cmd.Stderr = os.Stderr

	if err := p.cmd.Start(); err != nil {
		return errProcess.Wrap(err)
	}

	p.setLastRestarted(time.Now())

	return nil
}

// setLastRestarted sets the last restarted time.
func (p *Process) setLastRestarted(t time.Time) {
	p.mu.Lock()
	p.lastRestarted = t.UTC()
	p.mu.Unlock()
}

// lastRestartedTime returns the last restarted time.
func (p *Process) lastRestartedTime() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastRestarted.UTC()
}

// wait waits for the process to finish.
func (p *Process) wait() error {
	if p.cmd == nil {
		return nil
	}

	defer func() {
		p.cmd = nil
	}()

	return errProcess.Wrap(p.cmd.Wait())
}

// exit stops the process by sending an interrupt signal.
func (p *Process) exit() error {
	if p.cmd == nil {
		return nil
	}
	return errProcess.Wrap(p.cmd.Process.Signal(os.Interrupt))
}

// kill stops the process immediately.
func (p *Process) kill() error {
	if p.cmd == nil {
		return nil
	}
	return errProcess.Wrap(p.cmd.Process.Signal(os.Kill))
}

// Version returns the version of the process.
func (p *Process) Version(ctx context.Context) (version.SemVer, error) {
	return binaryVersion(ctx, p.binPath)
}

func parseVersion(out []byte) (version.SemVer, error) {
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		prefix := "Version: "
		if strings.HasPrefix(line, prefix) {
			line = line[len(prefix):]
			return version.NewSemVer(line)
		}
	}
	return version.SemVer{}, errs.New("unable to determine binary version")
}

func binaryVersion(ctx context.Context, location string) (version.SemVer, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	out, err := exec.CommandContext(ctx, location, "version").CombinedOutput()
	if err != nil {
		return version.SemVer{}, err
	}

	return parseVersion(out)
}
