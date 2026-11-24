package customercredituc_test

import (
	"context"
	"errors"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// absentTrustedMobile is a read-only test collaborator, not a production noop.
// It models a caller who does not appear in the offline imported trusted list.
type absentTrustedMobile struct{}

func (absentTrustedMobile) Conn(repo.Conn) repo.TrustedMobileQueryer {
	return absentTrustedMobile{}
}

func (absentTrustedMobile) Tx(repo.Tx) repo.TrustedMobileTxQueryer {
	return absentTrustedMobile{}
}

func (absentTrustedMobile) GetByMobile(context.Context, string) (model.TrustedMobileNumber, error) {
	return model.TrustedMobileNumber{}, cerr.ResourceNotFound(errors.New("trusted mobile not imported"))
}

func (absentTrustedMobile) InsertIfAbsent(
	context.Context,
	string,
	string,
	string,
	time.Time,
) (bool, error) {
	return false, errors.New("test-only trusted registry is read only")
}
