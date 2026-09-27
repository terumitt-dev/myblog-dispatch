package distlock

import (
	"context"
	"errors"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes/fake"
)

func TestLease_Acquire_FirstHolderSucceeds(t *testing.T) {
	client := fake.NewClientset()
	lock := New(client, "test-ns", "refresh-lock", "pod-a")

	if err := lock.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}
}

func TestLease_Acquire_SecondHolderBlockedWhileValid(t *testing.T) {
	client := fake.NewClientset()
	lockA := New(client, "test-ns", "refresh-lock", "pod-a")
	lockB := New(client, "test-ns", "refresh-lock", "pod-b")

	if err := lockA.Acquire(context.Background()); err != nil {
		t.Fatalf("pod-a Acquire returned error: %v", err)
	}

	err := lockB.Acquire(context.Background())
	if !errors.Is(err, ErrHeld) {
		t.Fatalf("pod-b Acquire error = %v, want ErrHeld", err)
	}
}

func TestLease_Acquire_TakesOverExpiredLease(t *testing.T) {
	client := fake.NewClientset()
	lockA := New(client, "test-ns", "refresh-lock", "pod-a")
	lockA.leaseDuration = 10 * time.Millisecond

	if err := lockA.Acquire(context.Background()); err != nil {
		t.Fatalf("pod-a Acquire returned error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	lockB := New(client, "test-ns", "refresh-lock", "pod-b")
	if err := lockB.Acquire(context.Background()); err != nil {
		t.Fatalf("pod-b Acquire over expired lease returned error: %v", err)
	}
}

func TestLease_Release(t *testing.T) {
	client := fake.NewClientset()
	lock := New(client, "test-ns", "refresh-lock", "pod-a")

	if err := lock.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}
	if err := lock.Release(context.Background()); err != nil {
		t.Fatalf("Release returned error: %v", err)
	}

	// 解放後は別 holder がすぐに取得できる。
	lockB := New(client, "test-ns", "refresh-lock", "pod-b")
	if err := lockB.Acquire(context.Background()); err != nil {
		t.Fatalf("pod-b Acquire after release returned error: %v", err)
	}
}

func TestLease_Release_AlreadyMissing(t *testing.T) {
	client := fake.NewClientset()
	lock := New(client, "test-ns", "refresh-lock", "pod-a")

	if err := lock.Release(context.Background()); err != nil {
		t.Fatalf("Release of missing lease returned error: %v", err)
	}
}

// TestLease_Release_DoesNotDeleteTakenOverLease は、自分の Lease が期限切れで
// 別 Pod に引き継がれた後に Release を呼んでも、その別 Pod のロックを
// 誤って削除しないことを検証する。
func TestLease_Release_DoesNotDeleteTakenOverLease(t *testing.T) {
	client := fake.NewClientset()
	lockA := New(client, "test-ns", "refresh-lock", "pod-a")
	lockA.leaseDuration = 10 * time.Millisecond

	if err := lockA.Acquire(context.Background()); err != nil {
		t.Fatalf("pod-a Acquire returned error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	lockB := New(client, "test-ns", "refresh-lock", "pod-b")
	if err := lockB.Acquire(context.Background()); err != nil {
		t.Fatalf("pod-b Acquire over expired lease returned error: %v", err)
	}

	// pod-a が処理完了後に (自分が引き継がれたことを知らずに) Release を呼ぶ。
	if err := lockA.Release(context.Background()); err != nil {
		t.Fatalf("pod-a Release returned error: %v", err)
	}

	// pod-b がまだロックを保持しているはず。pod-c は取得できない。
	lockC := New(client, "test-ns", "refresh-lock", "pod-c")
	err := lockC.Acquire(context.Background())
	if !errors.Is(err, ErrHeld) {
		t.Fatalf("pod-c Acquire error = %v, want ErrHeld (pod-a's Release should not have deleted pod-b's lease)", err)
	}
}
