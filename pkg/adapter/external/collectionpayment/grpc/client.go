// Package grpc implements the Financial Core CollectionPayment v2 client.
// The generated proto modules are intentionally kept in gen/proto; run
// "make proto" after upgrading the v2 schema. No caller-selected ledger scope
// or customer bearer token crosses this boundary.
package grpc

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
	grpcclient "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	collectionpb "github.com/parstis/bnpl-backend/gen/proto/financial/collectionpayment/v2"
	commonpb "github.com/parstis/bnpl-backend/gen/proto/financial/common/v1"
	"github.com/parstis/bnpl-backend/pkg/core/collectionpayment"
	external "github.com/parstis/bnpl-backend/pkg/core/external/collectionpayment"
)

type Client struct {
	api     collectionpb.CollectionPaymentServiceClient
	timeout time.Duration
}

var _ external.Client = (*Client)(nil)

func New(conn *grpcclient.ClientConn, timeout time.Duration) (*Client, error) {
	if conn == nil || timeout <= 0 {
		return nil, fmt.Errorf("collectionpayment/grpc.New: connection and positive timeout required")
	}
	return &Client{api: collectionpb.NewCollectionPaymentServiceClient(conn), timeout: timeout}, nil
}

// rpc propagates the caller's trusted correlation context. Financial Core
// requires this metadata for authenticated internal operations.
func (c *Client) rpc(ctx context.Context) (context.Context, context.CancelFunc, error) {
	id := strings.TrimSpace(trace.CorrelationID(ctx))
	if id == "" {
		return ctx, func() {}, fmt.Errorf("correlation id is required for CollectionPayment RPC")
	}
	ctx = metadata.AppendToOutgoingContext(ctx, "x-correlation-id", id)
	child, cancel := context.WithTimeout(ctx, c.timeout)
	return child, cancel, nil
}
func amount(n uint64, currency string) (*commonpb.Money, error) {
	if n > math.MaxInt64 {
		return nil, fmt.Errorf("amount overflows FC int64 minor units")
	}
	return &commonpb.Money{AmountMinor: int64(n), Currency: currency}, nil
}
func source(kind, id string, version int64) *commonpb.BusinessSourceReference {
	p := &commonpb.BusinessSourceReference{Type: kind, Id: id}
	if version > 0 {
		p.Version = &version
	}
	return p
}
func party(typ, id string) (*collectionpb.CollectionParty, error) {
	t := strings.ToLower(typ)
	var kind collectionpb.CollectionPartyType
	switch t {
	case "platform":
		kind = collectionpb.CollectionPartyType_COLLECTION_PARTY_TYPE_PLATFORM
	case "merchant":
		kind = collectionpb.CollectionPartyType_COLLECTION_PARTY_TYPE_MERCHANT
	case "funder":
		kind = collectionpb.CollectionPartyType_COLLECTION_PARTY_TYPE_FUNDER
	case "customer":
		kind = collectionpb.CollectionPartyType_COLLECTION_PARTY_TYPE_CUSTOMER
	case "tax_authority":
		kind = collectionpb.CollectionPartyType_COLLECTION_PARTY_TYPE_TAX_AUTHORITY
	default:
		return nil, fmt.Errorf("unsupported financial party type %q", typ)
	}
	p := &collectionpb.CollectionParty{Type: kind}
	if id != "" {
		p.PartyId = &id
	}
	if id == "" && (kind == collectionpb.CollectionPartyType_COLLECTION_PARTY_TYPE_MERCHANT ||
		kind == collectionpb.CollectionPartyType_COLLECTION_PARTY_TYPE_FUNDER ||
		kind == collectionpb.CollectionPartyType_COLLECTION_PARTY_TYPE_CUSTOMER) {
		return nil, fmt.Errorf("FC party id required for %s", t)
	}
	return p, nil
}
func assessment(a collectionpayment.Assessment) *collectionpb.CollectionAssessment {
	return &collectionpb.CollectionAssessment{
		Source:          source(a.SourceType, a.SourceID, a.SourceVersion),
		PolicyReference: a.PolicyReference, PolicyVersion: a.PolicyVersion,
		AssessedAt: timestamppb.New(a.AssessedAt),
	}
}
func componentType(kind collectionpayment.ComponentKind) (collectionpb.CollectionComponentType, error) {
	switch kind {
	case collectionpayment.PrincipalComponent:
		return collectionpb.CollectionComponentType_COLLECTION_COMPONENT_TYPE_PRINCIPAL, nil
	case collectionpayment.LateFeeComponent:
		return collectionpb.CollectionComponentType_COLLECTION_COMPONENT_TYPE_LATE_FEE, nil
	case collectionpayment.ServiceFeeComponent:
		return collectionpb.CollectionComponentType_COLLECTION_COMPONENT_TYPE_SERVICE_FEE, nil
	case collectionpayment.TaxComponent:
		return collectionpb.CollectionComponentType_COLLECTION_COMPONENT_TYPE_TAX, nil
	case collectionpayment.BaseAmountComponent:
		return collectionpb.CollectionComponentType_COLLECTION_COMPONENT_TYPE_BASE_AMOUNT, nil
	}
	return 0, fmt.Errorf("unknown component kind %q", kind)
}
func purpose(p collectionpayment.AllocationPurpose) (collectionpb.CollectionAllocationPurpose, error) {
	switch p {
	case collectionpayment.ReceivableRecoveryPurpose:
		return collectionpb.CollectionAllocationPurpose_COLLECTION_ALLOCATION_PURPOSE_RECEIVABLE_RECOVERY, nil
	case collectionpayment.MerchantPayablePurpose:
		return collectionpb.CollectionAllocationPurpose_COLLECTION_ALLOCATION_PURPOSE_MERCHANT_PAYABLE_ON_COLLECTION, nil
	case collectionpayment.PlatformRevenuePurpose:
		return collectionpb.CollectionAllocationPurpose_COLLECTION_ALLOCATION_PURPOSE_PLATFORM_REVENUE_ON_COLLECTION, nil
	case collectionpayment.FunderPayablePurpose:
		return collectionpb.CollectionAllocationPurpose_COLLECTION_ALLOCATION_PURPOSE_FUNDER_PAYABLE_ON_COLLECTION, nil
	case collectionpayment.TaxLiabilityPurpose:
		return collectionpb.CollectionAllocationPurpose_COLLECTION_ALLOCATION_PURPOSE_TAX_LIABILITY_ON_COLLECTION, nil
	case collectionpayment.PreRecognitionCashPurpose:
		return collectionpb.CollectionAllocationPurpose_COLLECTION_ALLOCATION_PURPOSE_PRE_RECOGNITION_CASH, nil
	}
	return 0, fmt.Errorf("unknown allocation purpose %q", p)
}
func fundingModel(m collectionpayment.FundingModel) (collectionpb.CollectionFundingModel, error) {
	switch m {
	case collectionpayment.FundingPlatform:
		return collectionpb.CollectionFundingModel_COLLECTION_FUNDING_MODEL_PLATFORM, nil
	case collectionpayment.FundingBank:
		return collectionpb.CollectionFundingModel_COLLECTION_FUNDING_MODEL_BANK, nil
	case collectionpayment.FundingMerchant:
		return collectionpb.CollectionFundingModel_COLLECTION_FUNDING_MODEL_MERCHANT, nil
	case collectionpayment.FundingCustomer:
		return collectionpb.CollectionFundingModel_COLLECTION_FUNDING_MODEL_CUSTOMER, nil
	}
	return 0, fmt.Errorf("unknown funding model %q", m)
}
func wirePlan(plan collectionpayment.Plan, key string) (*collectionpb.PrepareCollectionPaymentRequest, error) {
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("FC idempotency key required")
	}
	items := make([]*collectionpb.CollectionItem, 0, len(plan.Items))
	for _, item := range plan.Items {
		merchant := item.MerchantID.String()
		wire := &collectionpb.CollectionItem{
			ItemId: item.ID.String(), Source: source("bnpl_installment", item.InstallmentID.String(), 0),
			MerchantId: &merchant, BasisKind: collectionpb.CollectionBasisKind_COLLECTION_BASIS_KIND_INSTALLMENT,
		}
		for _, share := range item.FundingShares {
			m, err := fundingModel(share.Model)
			if err != nil {
				return nil, err
			}
			funder, err := party(share.Funder.Type, share.Funder.ID)
			if err != nil {
				return nil, err
			}
			wire.FundingShares = append(wire.FundingShares, &collectionpb.CollectionFundingShare{
				FundingShareId: share.ID.String(), Model: m, Funder: funder,
				FinancialRelationshipId: share.FinancialRelationshipID,
			})
		}
		for _, comp := range item.Components {
			typ, err := componentType(comp.Kind)
			if err != nil {
				return nil, err
			}
			gross, err := amount(comp.GrossMinor, string(plan.Currency))
			if err != nil {
				return nil, err
			}
			net, err := amount(comp.NetMinor, string(plan.Currency))
			if err != nil {
				return nil, err
			}
			cw := &collectionpb.CollectionComponent{
				ComponentId: comp.ID.String(), Type: typ,
				Assessment: assessment(comp.Assessment), GrossAmount: gross, NetAmount: net,
			}
			if comp.FinancialClaimID != "" {
				cw.FinancialClaimId = &comp.FinancialClaimID
			}
			if comp.TaxContext != nil {
				p, err := party(comp.TaxContext.LiablePartyType, comp.TaxContext.LiablePartyID)
				if err != nil {
					return nil, err
				}
				cw.TaxContext = &collectionpb.CollectionTaxContext{
					TaxCode: comp.TaxContext.TaxCode, Jurisdiction: comp.TaxContext.Jurisdiction,
					LiableParty: p, TaxAuthorityReference: comp.TaxContext.TaxAuthorityReference,
					TaxableBasisReference: comp.TaxContext.TaxableBasisReference,
					TreatmentReference:    comp.TaxContext.TreatmentReference,
					TreatmentVersion:      comp.TaxContext.TreatmentVersion,
				}
			}
			for _, adj := range comp.Adjustments {
				typ := collectionpb.CollectionAdjustmentType_COLLECTION_ADJUSTMENT_TYPE_DISCOUNT
				if adj.Kind == "waiver" {
					typ = collectionpb.CollectionAdjustmentType_COLLECTION_ADJUSTMENT_TYPE_WAIVER
				}
				m, err := amount(adj.AmountMinor, string(plan.Currency))
				if err != nil {
					return nil, err
				}
				cw.Adjustments = append(cw.Adjustments, &collectionpb.CollectionAdjustment{
					AdjustmentId: adj.ID.String(), Type: typ, Amount: m, Assessment: assessment(adj.Assessment),
				})
			}
			for _, alloc := range comp.Allocations {
				p, err := purpose(alloc.Purpose)
				if err != nil {
					return nil, err
				}
				beneficiary, err := party(alloc.BeneficiaryType, alloc.BeneficiaryID)
				if err != nil {
					return nil, err
				}
				m, err := amount(alloc.AmountMinor, string(plan.Currency))
				if err != nil {
					return nil, err
				}
				aw := &collectionpb.CollectionAllocation{
					AllocationId: alloc.ID.String(), Purpose: p, Beneficiary: beneficiary, Amount: m,
					DistributionRuleReference: alloc.RuleReference, DistributionRuleVersion: alloc.RuleVersion,
				}
				// Entitlement is NOT a recognition reference. Only the
				// authoritative FC-resolved recognition ID may be sent.
				if alloc.Purpose == collectionpayment.ReceivableRecoveryPurpose && alloc.RecognitionReference == "" {
					return nil, fmt.Errorf("receivable recovery requires FC-resolved recognition reference (entitlement %q)", alloc.EntitlementReference)
				}
				if alloc.RecognitionReference != "" {
					aw.RecognitionReference = &alloc.RecognitionReference
				}
				if alloc.FundingShareID != nil {
					v := alloc.FundingShareID.String()
					aw.FundingShareId = &v
				}
				cw.Allocations = append(cw.Allocations, aw)
			}
			wire.Components = append(wire.Components, cw)
		}
		items = append(items, wire)
	}
	m, err := amount(plan.TotalMinor, string(plan.Currency))
	if err != nil {
		return nil, err
	}
	return &collectionpb.PrepareCollectionPaymentRequest{
		Source:         source("bnpl_repayment_batch", plan.ID.String(), 1),
		IdempotencyKey: key, Items: items, PaymentAmount: m, Description: plan.Description,
		PayerCustomerId:   plan.ExternalCustomerID.String(),
		ObligorCustomerId: plan.ExternalCustomerID.String(),
	}, nil
}
func resultFromSnapshot(s *collectionpb.CollectionPaymentSnapshot) external.Result {
	if s == nil {
		return external.Result{}
	}
	r := external.Result{CollectionPaymentID: s.GetCollectionPaymentId(), SnapshotRevision: s.GetRevision(),
		Status: external.Prepared, ProviderOutcome: external.ProviderNotStarted, PostingOutcome: external.NotPosted}
	if src := s.GetSource(); src != nil {
		if id, err := uuid.Parse(src.GetId()); err == nil {
			r.SourceID = id
		}
	}
	return r
}
func parseStatus(s collectionpb.CollectionPaymentStatus) external.CollectionStatus {
	switch s {
	case collectionpb.CollectionPaymentStatus_COLLECTION_PAYMENT_STATUS_PREPARED:
		return external.Prepared
	case collectionpb.CollectionPaymentStatus_COLLECTION_PAYMENT_STATUS_STARTING:
		return external.Starting
	case collectionpb.CollectionPaymentStatus_COLLECTION_PAYMENT_STATUS_REQUIRES_CUSTOMER_ACTION:
		return external.RequiresCustomerAction
	case collectionpb.CollectionPaymentStatus_COLLECTION_PAYMENT_STATUS_RECONCILIATION_REQUIRED:
		return external.ReconciliationRequired
	case collectionpb.CollectionPaymentStatus_COLLECTION_PAYMENT_STATUS_ACCOUNTING_PENDING:
		return external.AccountingPending
	case collectionpb.CollectionPaymentStatus_COLLECTION_PAYMENT_STATUS_SUCCEEDED:
		return external.Succeeded
	case collectionpb.CollectionPaymentStatus_COLLECTION_PAYMENT_STATUS_FAILED:
		return external.Failed
	case collectionpb.CollectionPaymentStatus_COLLECTION_PAYMENT_STATUS_EXPIRED:
		return external.Expired
	case collectionpb.CollectionPaymentStatus_COLLECTION_PAYMENT_STATUS_BANK_REVERSED:
		return external.BankReversed
	}
	return ""
}
func parseProvider(s collectionpb.CollectionProviderOutcome) external.ProviderOutcome {
	switch s {
	case collectionpb.CollectionProviderOutcome_COLLECTION_PROVIDER_OUTCOME_NOT_STARTED:
		return external.ProviderNotStarted
	case collectionpb.CollectionProviderOutcome_COLLECTION_PROVIDER_OUTCOME_PENDING:
		return external.ProviderPending
	case collectionpb.CollectionProviderOutcome_COLLECTION_PROVIDER_OUTCOME_UNKNOWN:
		return external.ProviderUnknown
	case collectionpb.CollectionProviderOutcome_COLLECTION_PROVIDER_OUTCOME_SUCCEEDED:
		return external.ProviderSucceeded
	case collectionpb.CollectionProviderOutcome_COLLECTION_PROVIDER_OUTCOME_FAILED:
		return external.ProviderFailed
	case collectionpb.CollectionProviderOutcome_COLLECTION_PROVIDER_OUTCOME_REVERSED:
		return external.ProviderReversed
	}
	return ""
}
func parsePosting(s collectionpb.CollectionPostingOutcome) external.PostingOutcome {
	switch s {
	case collectionpb.CollectionPostingOutcome_COLLECTION_POSTING_OUTCOME_NOT_POSTED:
		return external.NotPosted
	case collectionpb.CollectionPostingOutcome_COLLECTION_POSTING_OUTCOME_PENDING:
		return external.PostingPending
	case collectionpb.CollectionPostingOutcome_COLLECTION_POSTING_OUTCOME_POSTED:
		return external.Posted
	case collectionpb.CollectionPostingOutcome_COLLECTION_POSTING_OUTCOME_REVERSAL_PENDING:
		return external.ReversalPending
	case collectionpb.CollectionPostingOutcome_COLLECTION_POSTING_OUTCOME_REVERSED:
		return external.Reversed
	}
	return ""
}
func parseResult(s *collectionpb.CollectionPaymentStatusResponse) (external.Result, error) {
	if s == nil || s.GetSnapshot() == nil {
		return external.Result{}, fmt.Errorf("FC collection response lacks immutable snapshot")
	}
	r := resultFromSnapshot(s.GetSnapshot())
	r.Status = parseStatus(s.GetStatus())
	r.ProviderOutcome = parseProvider(s.GetProviderOutcome())
	r.PostingOutcome = parsePosting(s.GetPostingOutcome())
	r.SnapshotRevision = s.GetSnapshot().GetRevision()
	r.PaymentIntentID = s.GetPaymentIntentId()
	r.PaymentSessionID = s.GetPaymentSessionId()
	r.PaymentLink = s.GetPaymentLink()
	r.ReasonCode = s.GetReasonCode()
	if r.Status == "" || r.ProviderOutcome == "" || r.PostingOutcome == "" {
		return external.Result{}, fmt.Errorf("unsupported/unspecified FC collection or provider/posting outcome")
	}
	if x := s.GetProviderPaidAt(); x != nil {
		t := x.AsTime()
		r.ProviderPaidAt = &t
	}
	if x := s.GetPostedAt(); x != nil {
		t := x.AsTime()
		r.PostedAt = &t
	}
	for _, p := range s.GetPostedAllocations() {
		item, err := uuid.Parse(p.GetItemId())
		if err != nil {
			return external.Result{}, fmt.Errorf("invalid posted item: %w", err)
		}
		component, err := uuid.Parse(p.GetComponentId())
		if err != nil {
			return external.Result{}, fmt.Errorf("invalid posted component: %w", err)
		}
		allocation, err := uuid.Parse(p.GetAllocationId())
		if err != nil {
			return external.Result{}, fmt.Errorf("invalid posted allocation: %w", err)
		}
		if p.GetAmount() == nil || p.GetAmount().GetCurrency() != s.GetSnapshot().GetPaymentAmount().GetCurrency() ||
			p.GetAmount().GetAmountMinor() <= 0 {
			return external.Result{}, fmt.Errorf("invalid FC posted allocation amount or currency")
		}
		r.PostedAllocations = append(r.PostedAllocations, external.PostedAllocation{
			ItemID: item, ComponentID: component, AllocationID: allocation,
			AmountMinor: uint64(p.GetAmount().GetAmountMinor()), PostingReference: p.GetPostingReference(),
		})
	}
	return r, nil
}
func (c *Client) Prepare(ctx context.Context, plan collectionpayment.Plan, key string) (external.Result, error) {
	req, err := wirePlan(plan, key)
	if err != nil {
		return external.Result{}, err
	}
	ctx, cancel, err := c.rpc(ctx)
	if err != nil {
		return external.Result{}, err
	}
	defer cancel()
	s, err := c.api.PrepareCollectionPayment(ctx, req)
	if err != nil {
		return external.Result{}, fmt.Errorf("prepare FC collection: %w", err)
	}
	r := resultFromSnapshot(s)
	if r.CollectionPaymentID == "" || r.SourceID != plan.ID || r.SnapshotRevision < 1 {
		return external.Result{}, fmt.Errorf("FC Prepare did not return matching durable snapshot")
	}
	return r, nil
}
func (c *Client) Start(ctx context.Context, id, key, provider string, urls external.ReturnURLs, revision int64) (external.Result, error) {
	if id == "" || key == "" || provider == "" || revision < 1 || !urls.IsComplete() {
		return external.Result{}, fmt.Errorf("invalid FC start parameters")
	}
	req := &collectionpb.StartCollectionPaymentRequest{
		CollectionPaymentId: id, IdempotencyKey: key, ProviderCode: provider,
		ReturnUrls:               &commonpb.PaymentReturnUrls{SuccessUrl: urls.Success, FailureUrl: urls.Failure, CancelUrl: urls.Cancel},
		ExpectedSnapshotRevision: revision,
	}
	ctx, cancel, metaErr := c.rpc(ctx)
	if metaErr != nil {
		return external.Result{}, metaErr
	}
	defer cancel()
	res, err := c.api.StartCollectionPayment(ctx, req)
	if err != nil {
		return external.Result{}, fmt.Errorf("start FC collection: %w", err)
	}
	return parseResult(res)
}
func (c *Client) GetByID(ctx context.Context, id string) (external.Result, error) {
	if id == "" {
		return external.Result{}, fmt.Errorf("FC collection id required")
	}
	ctx, cancel, metaErr := c.rpc(ctx)
	if metaErr != nil {
		return external.Result{}, metaErr
	}
	defer cancel()
	res, err := c.api.GetCollectionPayment(ctx, &collectionpb.GetCollectionPaymentRequest{
		Lookup: &collectionpb.GetCollectionPaymentRequest_CollectionPaymentId{CollectionPaymentId: id},
	})
	if err != nil {
		return external.Result{}, fmt.Errorf("get FC collection by ID: %w", err)
	}
	return parseResult(res)
}
func (c *Client) GetBySource(ctx context.Context, id uuid.UUID) (external.Result, error) {
	if id == uuid.Nil {
		return external.Result{}, fmt.Errorf("FC collection source id required")
	}
	ctx, cancel := c.rpc(ctx)
	defer cancel()
	res, err := c.api.GetCollectionPayment(ctx, &collectionpb.GetCollectionPaymentRequest{
		Lookup: &collectionpb.GetCollectionPaymentRequest_Source{
			Source: source("bnpl_repayment_batch", id.String(), 1),
		},
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return external.Result{}, cerr.ResourceNotFound(fmt.Errorf("FC collection source is not found: %w", err))
		}
		return external.Result{}, fmt.Errorf("get FC collection by source: %w", err)
	}
	return parseResult(res)
}
