package grpc

import (
	"fmt"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/parstis/bnpl-backend/pkg/core/external/merchant"
)

// isContractError keeps deterministic input, permission and version failures
// out of dependency-health counters. Transport failures still trip the breaker.
func isContractError(err error) bool {
	switch status.Code(err) {
	case codes.InvalidArgument, codes.Unauthenticated, codes.PermissionDenied,
		codes.NotFound, codes.FailedPrecondition, codes.AlreadyExists:
		return true

	default:
		return false
	}
}

func mapError(op string, err error) error {
	if err == nil {
		return nil
	}

	s, ok := status.FromError(err)
	if !ok {
		return fmt.Errorf("%s: merchant dependency failed: %w", op, err)
	}

	if s.Code() == codes.FailedPrecondition {
		for _, detail := range s.Details() {
			info, ok := detail.(*errdetails.ErrorInfo)
			if ok && info.GetDomain() == "parstis.merchant.participation.v1" &&
				info.GetReason() == "business_line.declaration_revision_conflict" {
				return fmt.Errorf("%s: %w", op, merchant.ErrDeclarationRevisionConflict)
			}
		}
	}

	if s.Code() == codes.NotFound {
		return fmt.Errorf("%s: %w", op, merchant.ErrReferenceNotFound)
	}

	if isContractError(err) {
		return fmt.Errorf("%s: %s: %w", op, s.Code(), merchant.ErrBlocked)
	}

	// Status messages may contain external information. Expose only the code.
	return fmt.Errorf("%s: merchant RPC failed: %s", op, s.Code())
}
