//go:build unit

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
	"testing"
)

// testAdder accumulates the sum of all previous testAdder outputs plus its own
// data value. It optionally fails on Run and Rollback when err is true.
type testAdder struct {
	data  int
	err   bool
	chain *Chain
}

func (tr *testAdder) Run(_ context.Context) (any, error) {
	foo := 0
	if otherData, ok := tr.chain.GetRunnerOutput(fmt.Sprintf("testTask%d", tr.data-1)); ok {
		foo = otherData.(int)
	}

	if tr.err {
		return tr.data, fmt.Errorf("bad news")
	}

	return tr.data + foo, nil
}

func (tr *testAdder) Rollback(_ context.Context) error {
	if tr.err {
		return fmt.Errorf("rollback bad news")
	}

	return nil
}

func (tr *testAdder) Name() string {
	return fmt.Sprintf("testTask%d", tr.data)
}

var basicTests = []struct {
	name        string
	testData    []int
	testFails   []bool
	testResults []int
}{
	{"4 commands - no error", []int{1, 2, 3, 4}, []bool{false, false, false, false}, []int{1, 3, 6, 10}},
	{"4 commands - error[1]", []int{1, 2, 3, 4}, []bool{false, true, false, false}, []int{1, -1, -1, -1}},
	{"5 commands - no error", []int{1, 2, 3, 4, 5}, []bool{false, false, false, false, false}, []int{1, 3, 6, 10, 15}},
	{"5 commands - error[3]", []int{1, 2, 3, 4, 5}, []bool{false, false, false, true, false}, []int{1, 3, 6, -1, -1}},
	{"6 commands - no error", []int{1, 2, 3, 4, 5, 6}, []bool{false, false, false, false, false, false}, []int{1, 3, 6, 10, 15, 21}},
	{"6 commands - error[3]", []int{1, 2, 3, 4, 5, 6}, []bool{false, false, false, true, false, false}, []int{1, 3, 6, -1, -1, -1}},
}

func TestBasic(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	for _, tc := range basicTests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			testChain := NewChain(2, 0)
			for i := range tc.testData {
				if err := testChain.AppendRunner(&testAdder{tc.testData[i], tc.testFails[i], testChain}); err != nil {
					t.Fatalf("%s: unexpected AppendRunner error: %v", tc.name, err)
				}
			}

			errorCheck(t, tc.name, tc.testFails, testChain, ctx)

			err := testChain.Execute(ctx)
			if err == nil {
				t.Fatalf("%s: should not be able to execute the same chain twice", tc.name)
			}

			if appendErr := testChain.AppendRunner(nil); !errors.Is(appendErr, ErrAlreadyExecuted) {
				t.Fatalf("%s: AppendRunner after Execute should return ErrAlreadyExecuted; got %v", tc.name, appendErr)
			}

			for i, result := range tc.testResults {
				name := fmt.Sprintf("testTask%d", i+1)
				out, ok := testChain.GetRunnerOutput(name)

				if result == -1 {
					if ok {
						t.Fatalf("%s: result for index %d should be absent; got %v", tc.name, i, out)
					}
				} else if !ok || out != result {
					t.Fatalf("%s: result for index %d should be %v; got %v (ok=%v)", tc.name, i, result, out, ok)
				}
			}
		})
	}
}

func TestMistakes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Two runners with the same name must be rejected.
	testChain := NewChain(0, 0)
	if err := testChain.AppendRunner(&testAdder{1, false, testChain}); err != nil {
		t.Fatalf("unexpected AppendRunner error: %v", err)
	}
	if err := testChain.AppendRunner(&testAdder{1, false, testChain}); err != nil {
		t.Fatalf("unexpected AppendRunner error: %v", err)
	}
	err := testChain.Execute(ctx)
	if err == nil {
		t.Fatalf("%s: should not be able to execute the chain with two runners named the same thing", "TestMistakes - samename")
	}

	// A nil runner followed by a failing runner must propagate the error.
	testChain = NewChain(0, 0)
	if err = testChain.AppendRunner(nil); err != nil {
		t.Fatalf("unexpected AppendRunner error: %v", err)
	}
	if err = testChain.AppendRunner(&testAdder{1, true, testChain}); err != nil {
		t.Fatalf("unexpected AppendRunner error: %v", err)
	}
	err = testChain.Execute(ctx)
	if err == nil {
		t.Fatalf("%s: should get an error for a chain with a nil runner with a failed command", "TestMistakes - nil task")
	}

	// A nil runner followed by a succeeding runner must not error.
	testChain = NewChain(0, 0)
	if err = testChain.AppendRunner(nil); err != nil {
		t.Fatalf("unexpected AppendRunner error: %v", err)
	}
	if err = testChain.AppendRunner(&testAdder{1, false, testChain}); err != nil {
		t.Fatalf("unexpected AppendRunner error: %v", err)
	}
	err = testChain.Execute(ctx)
	if err != nil {
		t.Fatalf("%s: should not get a error for a chain with a nil runner", "TestMistakes - nil task")
	}
}

// countRunner succeeds after failFor failed attempts, recording the total call count.
type countRunner struct {
	name    string
	calls   int
	failFor int
}

func (r *countRunner) Run(_ context.Context) (any, error) {
	r.calls++
	if r.calls <= r.failFor {
		return nil, fmt.Errorf("temporary error (attempt %d)", r.calls)
	}

	return r.calls, nil
}

func (r *countRunner) Rollback(_ context.Context) error { return nil }
func (r *countRunner) Name() string                     { return r.name }

func TestRetry(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("succeeds after retries", func(t *testing.T) {
		t.Parallel()

		runner := &countRunner{name: "retryable", failFor: 2}
		c := NewChain(2, 0, runner)

		if err := c.Execute(ctx); err != nil {
			t.Fatalf("expected success; got %v", err)
		}

		if runner.calls != 3 { // 1 initial + 2 retries
			t.Fatalf("expected 3 calls; got %d", runner.calls)
		}

		if out, ok := c.GetRunnerOutput("retryable"); !ok || out != 3 {
			t.Fatalf("expected output 3; got %v (ok=%v)", out, ok)
		}
	})

	t.Run("fails after exhausting retries", func(t *testing.T) {
		t.Parallel()

		runner := &countRunner{name: "always-fails", failFor: 10}
		c := NewChain(2, 0, runner)

		if err := c.Execute(ctx); err == nil {
			t.Fatal("expected error; got nil")
		}

		if runner.calls != 3 { // 1 initial + 2 retries
			t.Fatalf("expected 3 calls; got %d", runner.calls)
		}
	})
}

// trackRunner records whether it ran and was rolled back, optionally cancelling
// a context after Run completes.
type trackRunner struct {
	name   string
	ran    bool
	rolled bool
	cancel context.CancelFunc // if non-nil, called at the end of Run
}

func (r *trackRunner) Run(_ context.Context) (any, error) {
	r.ran = true
	if r.cancel != nil {
		r.cancel()
	}

	return nil, nil
}

func (r *trackRunner) Rollback(_ context.Context) error {
	r.rolled = true

	return nil
}

func (r *trackRunner) Name() string { return r.name }

func TestContextCancellation(t *testing.T) {
	t.Parallel()

	t.Run("pre-cancelled context rolls back nothing", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel before Execute

		r1 := &trackRunner{name: "r1"}
		r2 := &trackRunner{name: "r2"}
		c := NewChain(0, 0, r1, r2)

		if err := c.Execute(ctx); err == nil {
			t.Fatal("expected context error; got nil")
		}

		if r1.ran || r2.ran {
			t.Fatal("no runners should have run with a pre-cancelled context")
		}

		if r1.rolled || r2.rolled {
			t.Fatal("no runners should have been rolled back with a pre-cancelled context")
		}
	})

	t.Run("mid-chain cancellation rolls back only completed runners", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())

		r1 := &trackRunner{name: "r1", cancel: cancel} // cancels ctx after running
		r2 := &trackRunner{name: "r2"}
		c := NewChain(0, 0, r1, r2)

		if err := c.Execute(ctx); err == nil {
			t.Fatal("expected context error; got nil")
		}

		if !r1.ran {
			t.Fatal("r1 should have run")
		}

		if r2.ran {
			t.Fatal("r2 should not have run after context cancellation")
		}

		if !r1.rolled {
			t.Fatal("r1 should have been rolled back")
		}

		if r2.rolled {
			t.Fatal("r2 should not have been rolled back")
		}
	})
}

// errorCheck executes the chain and verifies whether it errors as expected.
func errorCheck(t *testing.T, name string, fails []bool, c *Chain, ctx context.Context) {
	t.Helper()

	err := c.Execute(ctx)

	shouldFail := false
	for _, fail := range fails {
		if fail {
			shouldFail = true

			break
		}
	}

	if shouldFail {
		if err == nil {
			t.Fatalf("%s: expected error; got nil", name)
		}

		if c.Error() != err {
			t.Fatalf("%s: chain.Error() (%v) should match returned error (%v)", name, c.Error(), err)
		}
	} else {
		if err != nil {
			t.Fatalf("%s: expected no error; got %v", name, err)
		}
	}
}
