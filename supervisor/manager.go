// Copyright (C) 2024 Storj Labs, Inc.
// See LICENSE for copying information

package supervisor

import (
	"context"
	"log/slog"
	"time"

	"github.com/zeebo/errs"
	"golang.org/x/sync/errgroup"

	"storj.io/common/sync2"
	"storj.io/common/version"
)

const maxRetries = 3

var errSupervisor = errs.Class("supervisor")

// Manager manages the storagenode process.
// It manages only one storagenode process.
type Manager struct {
	updater *Updater

	process *Process

	updaterLoop *sync2.Cycle

	disableAutoRestart bool
	disableAutoUpdate  bool
}

// New creates a new process Manager.
func New(updater *Updater, process *Process, checkInterval time.Duration, disableAutoRestart, disableAutoUpdate bool) *Manager {
	return &Manager{
		updater:            updater,
		process:            process,
		updaterLoop:        sync2.NewCycle(checkInterval),
		disableAutoRestart: disableAutoRestart,
		disableAutoUpdate:  disableAutoUpdate,
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
				slog.Error("Process exited with error", "error", err)
			} else {
				slog.Info("Process exited")
			}

			if s.disableAutoRestart {
				return err
			}
		}
	})

	if !s.disableAutoUpdate {
		group.Go(func() error {
			// wait for the node to run for a while before checking for updates.
			sync2.Sleep(ctx, 30*time.Second)
			// run the updater loop.
			// most of the errors are logged and ignored, so we don't exit the supervisor.
			var curVersion version.SemVer
			return s.updaterLoop.Run(ctx, func(ctx context.Context) (err error) {
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
					// exit the process to restart it with the new binary.
					return errSupervisor.Wrap(s.process.exit())
				}

				return nil
			})
		})
	}

	return group.Wait()
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
