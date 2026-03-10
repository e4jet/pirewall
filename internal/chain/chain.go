/*
(c) Copyright 2017 Hewlett Packard Enterprise Development LP

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

/*
(c) Copyright 2023 Eric Paul Forgette - changes since fork

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.

This work was forked from the following repository
https://github.com/hpe-storage/dory
*/

package chain

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	// ErrAlreadyExecuted is returned when Execute or AppendRunner is called after the chain has executed.
	ErrAlreadyExecuted = errors.New("chain has already executed")
	// ErrDuplicateRunner is returned when two Runners share the same Name.
	ErrDuplicateRunner = errors.New("chain runner names must be unique")
)

// Runner describes a struct that can be run and rolled back.
//
// If Run returns an error, Rollback is still called so the runner can clean up
// any partial state. Rollback implementations must therefore be safe to call
// even when Run did not complete successfully.
type Runner interface {
	// Name is used for logging and locating the output of a previously run Runner.
	Name() string
	// Run does the work and returns a value. If error is returned the chain fails (after retries).
	Run(ctx context.Context) (any, error)
	// Rollback is used to undo whatever Run() did. It is called even if Run returned an error.
	Rollback(ctx context.Context) error
}

// Chain is a set of Runners that will be executed sequentially.
type Chain struct {
	maxRetryOnError  int
	sleepBeforeRetry time.Duration
	commands         []Runner
	output           map[string]any
	// outputLock guards output. Runners may spawn goroutines that call
	// GetRunnerOutput concurrently while Execute is writing to it.
	outputLock sync.RWMutex
	// lastStep tracks the index of the last runner to have started execution.
	// Initialized to -1 so that a context cancellation before any runner runs
	// does not incorrectly roll back runners that never executed.
	lastStep    int
	err         error
	rollbackErr error
	runLock     sync.Mutex
	done        bool
}

// NewChain creates a new chain with the given runners pre-loaded.
// retries dictates how many times a Runner should be retried on error.
// retrySleep is how long to sleep before retrying a failed Runner.
func NewChain(retries int, retrySleep time.Duration, runners ...Runner) *Chain {
	c := &Chain{
		commands:         make([]Runner, 0, len(runners)),
		maxRetryOnError:  retries,
		sleepBeforeRetry: retrySleep,
		output:           make(map[string]any),
		lastStep:         -1,
	}
	c.commands = append(c.commands, runners...)

	return c
}

// AppendRunner appends a Runner to the Chain.
// It returns ErrAlreadyExecuted if the chain has already been executed.
func (c *Chain) AppendRunner(cmd Runner) error {
	c.runLock.Lock()
	defer c.runLock.Unlock()

	if c.done {
		return ErrAlreadyExecuted
	}

	c.commands = append(c.commands, cmd)

	return nil
}

// Execute runs the chain exactly once.
func (c *Chain) Execute(ctx context.Context) error {
	c.runLock.Lock()
	defer c.runLock.Unlock()

	err := c.setup()
	if err != nil {
		return err
	}

	c.done = true

	for i, command := range c.commands {
		if command == nil {
			continue
		}

		if ctx.Err() != nil {
			c.err = ctx.Err()

			break
		}

		c.lastStep = i

		var out any

		out, err = c.runWithRetry(ctx, command)
		if err != nil {
			c.err = err

			break
		}

		c.outputLock.Lock()
		c.output[command.Name()] = out
		c.outputLock.Unlock()
	}

	if c.err != nil {
		c.rollback(ctx)
	}

	return c.err
}

// Error returns the last error returned by a Runner.
func (c *Chain) Error() error {
	return c.err
}

// ErrorRollback returns the accumulated rollback errors from all failed Rollback calls.
func (c *Chain) ErrorRollback() error {
	return c.rollbackErr
}

// GetRunnerOutput returns the output stored by a named Runner and whether that
// Runner has completed. ok is false if the Runner has not yet run or does not exist.
// It is valid to pass *Chain to a Runner so that the Runner can reference
// the output of Runners that executed before it.
func (c *Chain) GetRunnerOutput(name string) (any, bool) {
	c.outputLock.RLock()
	defer c.outputLock.RUnlock()

	v, ok := c.output[name]

	return v, ok
}

// rollback calls Rollback in reverse order on every runner that started execution.
// Runners that never started (lastStep < 0) are skipped entirely.
func (c *Chain) rollback(ctx context.Context) {
	if c.lastStep < 0 {
		return
	}

	completed := c.commands[:c.lastStep+1]

	for i := len(completed) - 1; i >= 0; i-- {
		if completed[i] == nil {
			continue
		}

		if rbErr := c.rollbackWithRetry(ctx, completed[i]); rbErr != nil {
			c.rollbackErr = errors.Join(c.rollbackErr, rbErr)
		}
	}
}

func (c *Chain) setup() error {
	if c.done {
		return ErrAlreadyExecuted
	}

	seen := make(map[string]struct{}, len(c.commands))

	for _, command := range c.commands {
		if command == nil {
			continue
		}

		if _, found := seen[command.Name()]; found {
			return fmt.Errorf("runner %q: %w", command.Name(), ErrDuplicateRunner)
		}

		seen[command.Name()] = struct{}{}
	}

	return nil
}

func (c *Chain) runWithRetry(ctx context.Context, command Runner) (out any, err error) {
	for try := 0; try <= c.maxRetryOnError; try++ {
		out, err = command.Run(ctx)
		if err == nil {
			return out, nil
		}

		if try < c.maxRetryOnError {
			select {
			case <-ctx.Done():
				return out, ctx.Err()
			case <-time.After(c.sleepBeforeRetry):
			}
		}
	}

	return out, err
}

func (c *Chain) rollbackWithRetry(ctx context.Context, command Runner) (err error) {
	for try := 0; try <= c.maxRetryOnError; try++ {
		err = command.Rollback(ctx)
		if err == nil {
			return nil
		}

		if try < c.maxRetryOnError {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.sleepBeforeRetry):
			}
		}
	}

	return err
}
