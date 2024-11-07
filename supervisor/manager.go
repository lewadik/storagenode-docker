// Copyright (C) 2024 Storj Labs, Inc.
// See LICENSE for copying information

package supervisor

import (
	"context"
	"log/slog"
	"math/rand"
	"time"

	"github.com/zeebo/errs"
	"golang.org/x/sync/errgroup"

	"storj.io/common/sync2"
	"storj.io/common/version"
)

var errSupervisor = errs.Class("supervisor")

// Manager manages the storagenode process.
// It manages only one storagenode process.
type Manager struct {
	updater *Updater

	process *Process

	updaterLoop *sync2.Cycle

	config Config
}

type Config struct {
	CheckInterval               time.Duration `env:"STORJ_SUPERVISOR_UPDATE_CHECK_INTERVAL" default:"15m" description:"Interval in seconds to check for updates"`
	ProcessExitTimeout          time.Duration `env:"STORJ_SUPERVISOR_PROCESS_EXIT_TIMEOUT" default:"15s" description:"Timeout to wait for the process to exit; after this time, the process will be killed"`
	CheckMaxSleep               time.Duration `env:"STORJ_SUPERVISOR_UPDATE_CHECK_MAXIMUM_SLEEP" default:"300s" description:"maximum time to wait before checking for new update"`
	DisableProcessRestartOnExit bool          `env:"STORJ_SUPERVISOR_DISABLE_PROCESS_RESTART_ON_EXIT" default:"false" description:"Disable restarting the process when it exits. Useful for running storagenode setup command"`
	DisableAutoupdate           bool          `env:"STORJ_SUPERVISOR_DISABLE_AUTOUPDATE" default:"false" description:"Disable automatic updates"`
}

// New creates a new process Manager.
func New(updater *Updater, process *Process, config Config) *Manager {
	return &Manager{
		updater:     updater,
		process:     process,
		updaterLoop: sync2.NewCycle(config.CheckInterval),
		config:      config,
	}
}

// Run starts the supervisor
func (s *Manager) Run(ctx context.Context) error {
	group, ctx := errgroup.WithContext(ctx)

	group.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			slog.Info("Starting process", slog.String("binary", s.process.binPath))
			err := s.runProcess(ctx)
			if err != nil {
				slog.Warn("Process exited with error", "error", err)
			} else {
				slog.Info("Process exited")
			}

			if s.config.DisableProcessRestartOnExit {
				return err
			}
		}
	})

	if !s.config.DisableAutoupdate {
		group.Go(func() error {
			// run the updater loop.
			// most of the errors are logged and ignored, so we don't exit the supervisor.
			var curVersion version.SemVer
			return s.updaterLoop.Run(ctx, func(ctx context.Context) (err error) {
				// wait for a while before checking for updates.
				jitter := time.Duration(rand.Int63n(int64(s.config.CheckMaxSleep)))
				if !sync2.Sleep(ctx, jitter) {
					return errSupervisor.Wrap(ctx.Err())
				}

				if curVersion.IsZero() {
					curVersion, err = s.process.Version(ctx)
					if err != nil {
						slog.Error("Failed to get current version", "error", err)
						return nil
					}
				}

				newVersion, updated, err := s.updater.Update(ctx, s.process, curVersion)
				if err != nil {
					slog.Error("Failed to update process", "error", err)
					return nil
				}

				if updated {
					// reset the current version to force a new check.
					curVersion = newVersion
					return errSupervisor.Wrap(s.reapProcess(ctx))
				}

				return nil
			})
		})
	}

	return group.Wait()
}

// reapProcess tries to exit the process and waits for a few seconds for the process to exit,
// and then force-kills it if it takes too long to exit.
func (s *Manager) reapProcess(ctx context.Context) error {
	lastRestarted := s.process.lastRestartedTime()
	oldPID := s.process.pid()
	slog.Info("Exiting process", slog.Int("pid", oldPID))
	// exit the process to restart it with the new binary.
	if err := s.process.exit(); err != nil {
		return errSupervisor.Wrap(err)
	}
	// wait for the process to exit.
	if !sync2.Sleep(ctx, s.config.ProcessExitTimeout) {
		return ctx.Err()
	}
	// check if the process has exited.
	if s.process.pid() == 0 || s.process.pid() != oldPID {
		return nil
	}
	// for cases where the new process could be using the same PID as the old one,
	// we check if the process has been restarted since we sent the exit signal.
	if !s.process.lastRestartedTime().Equal(lastRestarted) {
		return nil
	}

	slog.Info("Process is taking too long to exit, killing it", slog.Int("pid", s.process.pid()))

	return s.process.kill()
}

func (s *Manager) runProcess(ctx context.Context) error {
	if err := s.process.start(ctx); err != nil {
		return err
	}

	return s.process.wait()
}

// Close stops all processes managed by the supervisor including the updater.
func (s *Manager) Close() error {
	s.updaterLoop.Close()
	return s.process.exit()
}
