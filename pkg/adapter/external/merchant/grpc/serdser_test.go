package grpc

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/parstis/bnpl-backend/pkg/core/external/merchant"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestProgressDeclarationRevision(t *testing.T) {
	p := merchant.Progress{
		RequestID:    model.MerchantParticipationRequestID(uuid.New()),
		EventID:      uuid.NewString(),
		OwnerVersion: 1,
		Status:       model.MerchantParticipationChangesRequested,
		Issues:       []model.MerchantParticipationIssue{{Field: "self_declaration", Code: "bnpl.declaration_required", Message: "Complete the declaration."}},
	}
	req, err := toProgressRequest(p)
	require.NoError(t, err)
	require.Nil(t, req.DeclarationRevision)

	p.DeclarationRevision = 2
	req, err = toProgressRequest(p)
	require.NoError(t, err)
	require.Equal(t, int64(2), req.GetDeclarationRevision())
}

func TestMapRevisionConflict(t *testing.T) {
	s, err := status.New(codes.FailedPrecondition, "conflict").WithDetails(&errdetails.ErrorInfo{
		Domain: "parstis.merchant.participation.v1",
		Reason: "business_line.declaration_revision_conflict",
	})
	require.NoError(t, err)
	require.ErrorIs(t, mapError("test", s.Err()), merchant.ErrDeclarationRevisionConflict)
	require.True(t, isContractError(s.Err()))
	require.False(t, isContractError(status.Error(codes.Unavailable, "unavailable")))
}
