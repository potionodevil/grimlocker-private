package lockdown_test

import (
	"sync"
	"testing"
	"time"

	"github.com/grimlocker/grimdb/security"
)

// TestHardLockdownTriggeredByThreshold verifies that concurrent authentication
// failures drive the module to LockdownHard and clear all MVK material.
func TestHardLockdownTriggeredByThreshold(t *testing.T) {
	exited := make(chan struct{})
	mod := security.NewModule(security.LockdownConfig{
		Threshold:       3,
		MaxOverrides:    1, // 1 override so 4 total failures → hard
		LockdownMinutes: 1,
	}, "").WithExitFunc(func(int) { close(exited) })

	handle, err := mod.StoreMVK([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("StoreMVK: %v", err)
	}

	// 3 concurrent failures to reach soft lockdown.
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mod.Lockdown().RecordFailure()
		}()
	}
	wg.Wait()

	// One more failure exhausts the single override → hard lockdown.
	mod.Lockdown().RecordFailure()

	// Wait for the asynchronous hard-lockdown callback.
	select {
	case <-exited:
	case <-time.After(2 * time.Second):
		t.Fatal("hard lockdown callback not invoked within 2s")
	}

	if got := mod.Lockdown().State(); got != security.LockdownHard {
		t.Errorf("expected LockdownHard, got %v", got)
	}

	if _, ok := mod.RetrieveMVK(handle); ok {
		t.Error("MVK handle should have been cleared by hard lockdown")
	}
}

// TestMVKWipeTiming verifies that the OnHard callback (which zeroes key material)
// is invoked within 5ms of the failure that triggers hard lockdown.
func TestMVKWipeTiming(t *testing.T) {
	wipeDone := make(chan time.Time, 1)

	// 32-byte mock key slice that the callback zeroes.
	mockKey := make([]byte, 32)
	copy(mockKey, []byte("deadbeefdeadbeefdeadbeefdeadbeef"))

	cfg := security.LockdownConfig{
		Threshold:       1,
		MaxOverrides:    0, // will be clamped to 4 by NewLockdownManager
		LockdownMinutes: 1,
		OnHard: func() {
			for i := range mockKey {
				mockKey[i] = 0
			}
			wipeDone <- time.Now()
		},
	}

	// Use LockdownManager directly so the OnHard is exactly our callback.
	// MaxOverrides gets clamped to 4; so after Threshold=1 failure → soft,
	// then 4 more failures exhaust overrides → hard.
	lm := security.NewLockdownManager(cfg)

	// Hit soft lockdown at threshold=1.
	lm.RecordFailure()

	// Exhaust all overrides to reach hard lockdown.
	before := time.Now()
	for i := 0; i < 4; i++ {
		lm.RecordFailure()
	}

	var wipeTime time.Time
	select {
	case wipeTime = <-wipeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("OnHard callback not invoked within 2s")
	}

	delta := wipeTime.Sub(before)
	t.Logf("MVK wipe latency: %v", delta)
	if delta > 5*time.Millisecond {
		t.Errorf("MVK wipe took %v; must be < 5ms", delta)
	}

	// Verify the key slice was actually zeroed.
	for i, b := range mockKey {
		if b != 0 {
			t.Errorf("mockKey[%d] = %d after wipe, expected 0", i, b)
		}
	}
}

// TestParallelAuthFailInjection fires 3 goroutines simultaneously and asserts
// that the lockdown state reaches at least LockdownSoft after the threshold.
func TestParallelAuthFailInjection(t *testing.T) {
	mod := security.NewModule(security.LockdownConfig{
		Threshold:       3,
		MaxOverrides:    4,
		LockdownMinutes: 1,
	}, "").WithExitFunc(func(int) {})

	ready := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ready // align goroutine starts
			mod.Lockdown().RecordFailure()
		}()
	}

	close(ready)
	wg.Wait()

	state := mod.Lockdown().State()
	if state < security.LockdownSoft {
		t.Errorf("expected at least LockdownSoft after 3 concurrent failures, got %v", state)
	}
}
