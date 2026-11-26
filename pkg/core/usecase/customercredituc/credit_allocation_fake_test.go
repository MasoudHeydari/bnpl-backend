package customercredituc_test

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// disabledCreditAllocation is a construct-time test collaborator that keeps
// public products disabled so AssignInitialLimit unit tests can still construct
// the use case without exercising the grant worker.
type disabledCreditAllocation struct{}

func (disabledCreditAllocation) Tx(repo.Tx) repo.CreditAllocationTxQueryer {
	return disabledCreditAllocationTx{}
}

type disabledCreditAllocationTx struct{}

func (disabledCreditAllocationTx) LockConfiguration(context.Context, bool) error {
	return nil
}

func (disabledCreditAllocationTx) GetGroup(context.Context, string) (model.CreditGroup, error) {
	return model.CreditGroup{}, cerr.ResourceNotFound(errors.New("missing group"))
}

func (disabledCreditAllocationTx) ListGroups(context.Context) ([]model.CreditGroup, error) {
	return nil, nil
}

func (disabledCreditAllocationTx) PutGroup(context.Context, model.CreditGroup) (model.CreditGroup, error) {
	return model.CreditGroup{}, cerr.InternalError(errors.New("unused"))
}

func (disabledCreditAllocationTx) GetEligibilityByNationalCode(context.Context, string) (model.CreditEligibilityEntry, error) {
	return model.CreditEligibilityEntry{}, cerr.ResourceNotFound(errors.New("missing eligibility"))
}

func (disabledCreditAllocationTx) GetEligibilityByMobile(context.Context, string) (model.CreditEligibilityEntry, error) {
	return model.CreditEligibilityEntry{}, cerr.ResourceNotFound(errors.New("missing eligibility"))
}

func (disabledCreditAllocationTx) GetEligibility(context.Context, uuid.UUID) (model.CreditEligibilityEntry, error) {
	return model.CreditEligibilityEntry{}, cerr.ResourceNotFound(errors.New("missing eligibility"))
}

func (disabledCreditAllocationTx) InsertEligibility(context.Context, model.CreditEligibilityEntry) (model.CreditEligibilityEntry, error) {
	return model.CreditEligibilityEntry{}, cerr.InternalError(errors.New("unused"))
}

func (disabledCreditAllocationTx) InsertEligibilityMobile(context.Context, model.CreditEligibilityMobile) error {
	return cerr.InternalError(errors.New("unused"))
}

func (disabledCreditAllocationTx) ListEligibility(context.Context, *string, *uuid.UUID, int) ([]model.CreditEligibilityEntry, error) {
	return nil, nil
}

func (disabledCreditAllocationTx) ListEligibilityMobiles(context.Context, uuid.UUID) ([]model.CreditEligibilityMobile, error) {
	return nil, nil
}

func (disabledCreditAllocationTx) BindEligibility(context.Context, uuid.UUID, model.CreditOwnerID) error {
	return nil
}

func (disabledCreditAllocationTx) GetMembership(context.Context, uuid.UUID) (model.CreditGroupMembership, error) {
	return model.CreditGroupMembership{}, cerr.ResourceNotFound(errors.New("missing membership"))
}

func (disabledCreditAllocationTx) PutMembership(context.Context, model.CreditGroupMembership) (model.CreditGroupMembership, error) {
	return model.CreditGroupMembership{}, cerr.InternalError(errors.New("unused"))
}

func (disabledCreditAllocationTx) GetRule(
	_ context.Context,
	scope model.CreditAllocationScope,
	_ *string,
	_ *uuid.UUID,
	kind model.CreditKind,
	currency model.Currency,
) (model.CreditAllocationRule, error) {
	if scope == model.DefaultCreditAllocationScope {
		return model.CreditAllocationRule{
			ID:         model.CreditAllocationRuleID(uuid.MustParse("11111111-1111-1111-1111-111111111111")),
			Scope:      model.DefaultCreditAllocationScope,
			CreditKind: kind,
			Currency:   currency,
			Strategy:   model.DisabledCreditAllocationStrategy,
			Version:    1,
		}, nil
	}

	return model.CreditAllocationRule{}, cerr.ResourceNotFound(errors.New("missing rule"))
}

func (disabledCreditAllocationTx) PutRule(context.Context, model.CreditAllocationRule) (model.CreditAllocationRule, error) {
	return model.CreditAllocationRule{}, cerr.InternalError(errors.New("unused"))
}

func (disabledCreditAllocationTx) ListRules(context.Context) ([]model.CreditAllocationRule, error) {
	return nil, nil
}

func (disabledCreditAllocationTx) GetOpenRequest(context.Context, model.CreditOwnerID, model.CreditKind, model.Currency) (model.CreditRequest, error) {
	return model.CreditRequest{}, cerr.ResourceNotFound(errors.New("no open request"))
}

func (disabledCreditAllocationTx) GetLatestRequest(context.Context, model.CustomerID, model.CreditKind, model.Currency) (model.CreditRequest, error) {
	return model.CreditRequest{}, cerr.ResourceNotFound(errors.New("no request"))
}

func (disabledCreditAllocationTx) GetRequest(context.Context, model.CreditRequestID, bool) (model.CreditRequest, error) {
	return model.CreditRequest{}, cerr.ResourceNotFound(errors.New("no request"))
}

func (disabledCreditAllocationTx) InsertRequest(_ context.Context, r model.CreditRequest) (model.CreditRequest, error) {
	r.ID = model.CreditRequestID(uuid.MustParse("22222222-2222-2222-2222-222222222222"))
	r.Status = model.PendingCreditRequestStatus
	r.UpdatedAt = r.RequestedAt
	return r, nil
}

func (disabledCreditAllocationTx) QueueIdentityRecheck(context.Context, model.CreditRequestID, time.Time) error {
	return nil
}

func (disabledCreditAllocationTx) ClaimRequest(context.Context, model.CreditRequestID, time.Time, time.Time) (model.CreditRequest, error) {
	return model.CreditRequest{}, cerr.ResourceNotFound(errors.New("nothing to claim"))
}

func (disabledCreditAllocationTx) QueuePublicRequests(context.Context, time.Time, int) error {
	return nil
}

func (disabledCreditAllocationTx) ListDueRequests(context.Context, time.Time, int) ([]model.CreditRequest, error) {
	return nil, nil
}

func (disabledCreditAllocationTx) FinishRequest(
	context.Context,
	model.CreditRequest,
	model.CreditRequestStatus,
	string,
	*model.DecisionID,
	*model.CreditAllocationRuleID,
	*time.Time,
	time.Time,
) (model.CreditRequest, error) {
	return model.CreditRequest{}, cerr.ResourceNotFound(errors.New("unused"))
}

func (disabledCreditAllocationTx) QueueEligibility(context.Context, *uuid.UUID, *string, *model.CreditKind, *model.Currency, time.Time) error {
	return nil
}

func (disabledCreditAllocationTx) ListPendingRequests(context.Context, model.CreditKind, time.Time, int) ([]model.CreditRequest, error) {
	return nil, nil
}
