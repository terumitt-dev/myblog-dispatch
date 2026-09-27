// Package distlock は k8s の Lease リソースを使い、複数 Pod（レプリカ）間で
// 排他制御を行うためのシンプルなクラスタ全体ロックを提供する。
//
// X の Refresh Token はローテーション方式（1回使うと無効化）のため、
// 複数 Pod が同時に同じ Refresh Token でリフレッシュを試みると、
// 片方が invalid_grant で失敗する。Lease をミューテックスとして使い、
// リフレッシュ処理をクラスタ全体で直列化する。
package distlock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ErrHeld は他の Pod が有効なロックを保持している場合に返される。
var ErrHeld = errors.New("lease lock is held by another holder")

const defaultLeaseDuration = 30 * time.Second

// Lease は coordination.k8s.io/v1 の Lease オブジェクトを使ったロック。
type Lease struct {
	client        kubernetes.Interface
	namespace     string
	name          string
	holderID      string
	leaseDuration time.Duration
}

// New は Lease ロックを構築する。holderID を空にすると HOSTNAME 環境変数
// (Pod 名) を使う。
func New(client kubernetes.Interface, namespace, name, holderID string) *Lease {
	if holderID == "" {
		holderID = os.Getenv("HOSTNAME")
	}
	if holderID == "" {
		holderID = "unknown"
	}
	return &Lease{
		client:        client,
		namespace:     namespace,
		name:          name,
		holderID:      holderID,
		leaseDuration: defaultLeaseDuration,
	}
}

// Acquire はロックの取得を試みる。他の holder が有効なロックを保持していれば
// ErrHeld を返す。
func (l *Lease) Acquire(ctx context.Context) error {
	now := metav1.NewMicroTime(time.Now())
	durationSec := int32(l.leaseDuration.Seconds())

	leases := l.client.CoordinationV1().Leases(l.namespace)
	existing, err := leases.Get(ctx, l.name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		lease := &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{Name: l.name, Namespace: l.namespace},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity:       &l.holderID,
				LeaseDurationSeconds: &durationSec,
				AcquireTime:          &now,
				RenewTime:            &now,
			},
		}
		if _, err := leases.Create(ctx, lease, metav1.CreateOptions{}); err != nil {
			if apierrors.IsAlreadyExists(err) {
				return ErrHeld
			}
			return fmt.Errorf("create lease: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get lease: %w", err)
	}

	// 有効期限は Lease に記録されている値（保持者が設定したもの）を使う。
	// 自分の leaseDuration ではなく、現在の保持者の申告した期間で判定する。
	recordedDuration := l.leaseDuration
	if existing.Spec.LeaseDurationSeconds != nil {
		recordedDuration = time.Duration(*existing.Spec.LeaseDurationSeconds) * time.Second
	}
	expired := existing.Spec.RenewTime == nil ||
		existing.Spec.RenewTime.Add(recordedDuration).Before(time.Now())
	heldByOther := existing.Spec.HolderIdentity != nil && *existing.Spec.HolderIdentity != l.holderID

	if heldByOther && !expired {
		return ErrHeld
	}

	existing.Spec.HolderIdentity = &l.holderID
	existing.Spec.LeaseDurationSeconds = &durationSec
	existing.Spec.RenewTime = &now
	if expired || existing.Spec.AcquireTime == nil {
		existing.Spec.AcquireTime = &now
	}

	if _, err := leases.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		if apierrors.IsConflict(err) {
			return ErrHeld
		}
		return fmt.Errorf("update lease: %w", err)
	}
	return nil
}

// Release はロックを解放する（Lease オブジェクトを削除する）。
func (l *Lease) Release(ctx context.Context) error {
	err := l.client.CoordinationV1().Leases(l.namespace).Delete(ctx, l.name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete lease: %w", err)
	}
	return nil
}
