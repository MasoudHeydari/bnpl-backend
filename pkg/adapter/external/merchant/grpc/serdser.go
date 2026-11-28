package grpc

import (
	"fmt"

	merchantv1 "github.com/parstis/bnpl-backend/gen/proto/merchant/participation/v1"
	"github.com/parstis/bnpl-backend/pkg/core/external/merchant"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func toReference(response *merchantv1.GetParticipationRequestResponse) (merchant.Reference, error) {
	const op = "grpc.toReference"

	if response == nil {
		return merchant.Reference{}, fmt.Errorf("%s: empty response: %w", op, merchant.ErrBlocked)
	}

	merchantID, err := model.ParseExternalMerchantID(response.GetMerchantId())
	if err != nil || merchantID.IsZero() || merchantID.String() != response.GetMerchantId() {
		return merchant.Reference{}, fmt.Errorf("%s: invalid merchant id: %w", op, merchant.ErrBlocked)
	}

	applicationID, err := model.ParseMerchantApplicationID(response.GetApplicationId())
	if err != nil || applicationID.IsZero() || applicationID.String() != response.GetApplicationId() {
		return merchant.Reference{}, fmt.Errorf("%s: invalid application id: %w", op, merchant.ErrBlocked)
	}

	snapshot, err := toSnapshot(response.GetParticipation())
	if err != nil {
		return merchant.Reference{}, fmt.Errorf("%s: %w", op, err)
	}

	var declaration *model.MerchantSelfDeclaration
	if d := response.GetSelfDeclaration(); d != nil {
		declaration = &model.MerchantSelfDeclaration{
			MaxAmountMinor:    d.GetMaxAmountMinor(),
			MaxAmountCurrency: model.Currency(d.GetMaxAmountCurrency()),
			MaxTenureMonths:   uint(d.GetMaxTenureMonths()),
		}
	}

	return merchant.Reference{
		MerchantID:            merchantID,
		ApplicationID:         applicationID,
		RequestID:             snapshot.RequestID,
		BusinessLine:          snapshot.BusinessLine,
		Environment:           snapshot.Environment,
		TradeName:             response.GetTradeName(),
		BaseApplicationStatus: response.GetBaseApplicationStatus(),
		Status:                snapshot.Status,
		OwnerVersion:          snapshot.OwnerVersion,
		DeclarationRevision:   snapshot.DeclarationRevision,
		SelfDeclaration:       declaration,
	}, nil
}

func toSnapshot(response *merchantv1.ParticipationSnapshot) (merchant.Snapshot, error) {
	const op = "grpc.toSnapshot"

	if response == nil || response.GetOwnerVersion() < 0 || response.GetDeclarationRevision() < 0 {
		return merchant.Snapshot{}, fmt.Errorf("%s: invalid participation snapshot: %w", op, merchant.ErrBlocked)
	}

	id, err := model.ParseMerchantParticipationRequestID(response.GetRequestId())
	if err != nil || id.IsZero() || id.String() != response.GetRequestId() {
		return merchant.Snapshot{}, fmt.Errorf("%s: invalid request id: %w", op, merchant.ErrBlocked)
	}

	environment, err := mapEnvironment(response.GetEnvironment())
	if err != nil {
		return merchant.Snapshot{}, fmt.Errorf("%s: %w", op, err)
	}

	state, err := mapStatus(response.GetStatus())
	if err != nil {
		return merchant.Snapshot{}, fmt.Errorf("%s: %w", op, err)
	}

	issues := make([]model.MerchantParticipationIssue, 0, len(response.GetIssues()))
	for _, issue := range response.GetIssues() {
		if issue == nil {
			return merchant.Snapshot{}, fmt.Errorf("%s: invalid issue: %w", op, merchant.ErrBlocked)
		}

		issues = append(issues, model.MerchantParticipationIssue{
			Field:   issue.GetField(),
			Code:    issue.GetCode(),
			Message: issue.GetMessage(),
		})
	}

	return merchant.Snapshot{
		RequestID:           id,
		BusinessLine:        response.GetBusinessLineCode(),
		Environment:         environment,
		Status:              state,
		OwnerVersion:        response.GetOwnerVersion(),
		DeclarationRevision: response.GetDeclarationRevision(),
		AccountReference:    response.GetExternalAccountReference(),
		Issues:              issues,
	}, nil
}

func toProgressRequest(p merchant.Progress) (*merchantv1.ReportParticipationProgressRequest, error) {
	const op = "grpc.toProgressRequest"

	if p.RequestID.IsZero() || p.OwnerVersion <= 0 || p.DeclarationRevision < 0 || p.EventID == "" {
		return nil, fmt.Errorf("%s: invalid progress identity: %w", op, merchant.ErrBlocked)
	}

	request := &merchantv1.ReportParticipationProgressRequest{
		RequestId:    p.RequestID.String(),
		EventId:      p.EventID,
		OwnerVersion: p.OwnerVersion,
	}
	switch p.Status {
	case model.MerchantParticipationReady:
		if p.AccountReference == "" || p.DeclarationRevision <= 0 || len(p.Issues) != 0 {
			return nil, fmt.Errorf("%s: invalid readiness report: %w", op, merchant.ErrBlocked)
		}

		request.Status = merchantv1.ParticipationStatus_PARTICIPATION_STATUS_READY
		request.ExternalAccountReference = &p.AccountReference

	case model.MerchantParticipationChangesRequested:
		if len(p.Issues) == 0 || p.AccountReference != "" {
			return nil, fmt.Errorf("%s: invalid changes requested report: %w", op, merchant.ErrBlocked)
		}

		request.Status = merchantv1.ParticipationStatus_PARTICIPATION_STATUS_CHANGES_REQUESTED

	default:
		return nil, fmt.Errorf("%s: invalid report status: %w", op, merchant.ErrBlocked)
	}

	if p.DeclarationRevision > 0 {
		request.DeclarationRevision = &p.DeclarationRevision
	}

	for _, issue := range p.Issues {
		request.Issues = append(request.Issues, &merchantv1.ParticipationIssue{
			Field:   issue.Field,
			Code:    issue.Code,
			Message: issue.Message,
		})
	}

	return request, nil
}

func mapEnvironment(value merchantv1.ParticipationEnvironment) (string, error) {
	switch value {
	case merchantv1.ParticipationEnvironment_PARTICIPATION_ENVIRONMENT_SANDBOX:
		return "SANDBOX", nil

	case merchantv1.ParticipationEnvironment_PARTICIPATION_ENVIRONMENT_LIVE:
		return "LIVE", nil

	default:
		return "", fmt.Errorf("grpc.mapEnvironment: unknown environment: %w", merchant.ErrBlocked)
	}
}

func mapStatus(value merchantv1.ParticipationStatus) (string, error) {
	switch value {
	case merchantv1.ParticipationStatus_PARTICIPATION_STATUS_REQUESTED:
		return "REQUESTED", nil

	case merchantv1.ParticipationStatus_PARTICIPATION_STATUS_UNDER_REVIEW:
		return "UNDER_REVIEW", nil

	case merchantv1.ParticipationStatus_PARTICIPATION_STATUS_CHANGES_REQUESTED:
		return model.MerchantParticipationChangesRequested, nil

	case merchantv1.ParticipationStatus_PARTICIPATION_STATUS_ACCEPTED:
		return "ACCEPTED", nil

	case merchantv1.ParticipationStatus_PARTICIPATION_STATUS_PROVISIONING:
		return "PROVISIONING", nil

	case merchantv1.ParticipationStatus_PARTICIPATION_STATUS_READY:
		return model.MerchantParticipationReady, nil

	case merchantv1.ParticipationStatus_PARTICIPATION_STATUS_REJECTED:
		return "REJECTED", nil

	case merchantv1.ParticipationStatus_PARTICIPATION_STATUS_SUSPENDED:
		return "SUSPENDED", nil

	default:
		return "", fmt.Errorf("grpc.mapStatus: unknown status: %w", merchant.ErrBlocked)
	}
}
