package admission

import (
	"context"
	"errors"
	"testing"
)

func TestInteractiveReserveKeepsCapacityForLatencySensitiveTraffic(t *testing.T) {
	controller := New(Config{MaxRunning: 2, InteractiveReserve: 1, MaxQueuedTokens: 100})
	batchLease, _, err := controller.Acquire(context.Background(), Batch, 10)
	if err != nil {
		t.Fatalf("acquire first batch request: %v", err)
	}
	defer batchLease.Release()

	if _, _, err := controller.Acquire(context.Background(), Batch, 10); !errors.Is(err, ErrShed) {
		t.Fatalf("second batch should shed immediately, got %v", err)
	}

	interactiveLease, _, err := controller.Acquire(context.Background(), Interactive, 10)
	if err != nil {
		t.Fatalf("reserved interactive capacity was unavailable: %v", err)
	}
	interactiveLease.Release()
}

func TestQueuedTokenBudgetShedsOversizedWork(t *testing.T) {
	controller := New(Config{MaxRunning: 1, MaxQueuedTokens: 12})
	lease, _, err := controller.Acquire(context.Background(), Interactive, 5)
	if err != nil {
		t.Fatalf("acquire running request: %v", err)
	}
	defer lease.Release()

	if _, _, err := controller.Acquire(context.Background(), Interactive, 13); !errors.Is(err, ErrShed) {
		t.Fatalf("oversized queued request should shed, got %v", err)
	}
}

func TestCanceledWaiterLeavesNoQueuedTokens(t *testing.T) {
	controller := New(Config{MaxRunning: 1, MaxQueuedTokens: 100})
	lease, _, err := controller.Acquire(context.Background(), Interactive, 5)
	if err != nil {
		t.Fatalf("acquire running request: %v", err)
	}
	defer lease.Release()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := controller.Acquire(ctx, Interactive, 30); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request returned %v", err)
	}
	if snapshot := controller.Snapshot(); snapshot.QueuedTokens != 0 || snapshot.QueuedRequests != 0 {
		t.Fatalf("canceled waiter leaked queue state: %+v", snapshot)
	}
}
