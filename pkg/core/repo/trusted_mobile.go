package repo

import (
	"context"
	"errors"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// ErrTrustedMobileNationalIDConflict prevents a previously imported phone
// from silently changing its national ID.
var ErrTrustedMobileNationalIDConflict = errors.New("trusted mobile national ID conflict")

// TrustedMobileQueryer looks up an imported mobile/national ID pair without
// involving the external Customer Reference Service.
type TrustedMobileQueryer interface {
	GetByMobile(ctx context.Context, mobileE164 string) (model.TrustedMobileNumber, error)
}

// TrustedMobileTxQueryer can atomically insert the immutable pair. An existing
// identical pair is idempotent; reusing a mobile with another national ID is a
// conflict, never an implicit update.
type TrustedMobileTxQueryer interface {
	TrustedMobileQueryer
	InsertIfAbsent(ctx context.Context, mobileE164, nationalID, createdBy string, createdAt time.Time) (created bool, err error)
}

// TrustedMobile is the persistence boundary for the trusted mobile registry.
type TrustedMobile interface {
	Conn(Conn) TrustedMobileQueryer
	Tx(Tx) TrustedMobileTxQueryer
}
