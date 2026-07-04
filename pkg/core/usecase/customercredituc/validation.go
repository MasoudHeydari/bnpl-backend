package customercredituc

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const (
	maxCreditPolicyIDLength      = 128
	maxCreditPolicyVersionLength = 64
	maxCreditReasonCodeLength    = 128
	maxCreditReasonCodes         = 32
)

func normalizeAndValidateInitialLimitAssessment(assessment InitialLimitAssessmentResult, now time.Time) (InitialLimitAssessmentResult, error) {
	const op = "customercredituc.normalizeAndValidateInitialLimitAssessment"

	assessment.PolicyID = strings.TrimSpace(assessment.PolicyID)
	assessment.PolicyVersion = strings.TrimSpace(assessment.PolicyVersion)

	if assessment.PolicyID == "" || len(assessment.PolicyID) > maxCreditPolicyIDLength {
		return InitialLimitAssessmentResult{}, creditAssessmentUnavailableError(
			errors.New("credit assessment returned invalid policy id"),
		)
	}

	if assessment.PolicyVersion == "" || len(assessment.PolicyVersion) > maxCreditPolicyVersionLength {
		return InitialLimitAssessmentResult{}, creditAssessmentUnavailableError(
			errors.New("credit assessment returned invalid policy version"),
		)
	}

	if len(assessment.ReasonCodes) == 0 || len(assessment.ReasonCodes) > maxCreditReasonCodes {
		return InitialLimitAssessmentResult{}, creditAssessmentUnavailableError(
			errors.New("credit assessment returned invalid reason codes"),
		)
	}

	for i, reasonCode := range assessment.ReasonCodes {
		reasonCode = strings.TrimSpace(reasonCode)
		if reasonCode == "" || len(reasonCode) > maxCreditReasonCodeLength {
			return InitialLimitAssessmentResult{}, creditAssessmentUnavailableError(
				errors.New("credit assessment returned invalid reason code"),
			)
		}

		assessment.ReasonCodes[i] = reasonCode
	}

	switch assessment.Outcome {
	case model.DeniedDecisionOutcome:
		if assessment.AmountMinor != 0 || assessment.ValidFor != 0 {
			return InitialLimitAssessmentResult{}, creditAssessmentUnavailableError(
				errors.New("denied credit assessment must not contain an approved amount or validity"),
			)
		}

		if !assessment.Currency.IsValid() {
			return InitialLimitAssessmentResult{}, creditAssessmentUnavailableError(
				errors.New("denied credit assessment returned invalid currency"),
			)
		}

	case model.LimitSetDecisionOutcome:
		if err := model.ValidatePositiveAmountMinor(assessment.AmountMinor); err != nil {
			return InitialLimitAssessmentResult{}, creditAssessmentUnavailableError(
				fmt.Errorf("%s: credit assessment amount: %w", op, err),
			)
		}

		if !assessment.Currency.IsValid() {
			return InitialLimitAssessmentResult{}, creditAssessmentUnavailableError(
				errors.New("credit assessment returned invalid currency"),
			)
		}

		if assessment.ValidFor <= 0 {
			return InitialLimitAssessmentResult{}, creditAssessmentUnavailableError(
				errors.New("credit assessment validity must be positive"),
			)
		}

		if now.Add(assessment.ValidFor).Before(now) {
			return InitialLimitAssessmentResult{}, creditAssessmentUnavailableError(
				errors.New("credit assessment validity overflows time"),
			)
		}

	default:
		return InitialLimitAssessmentResult{}, creditAssessmentUnavailableError(
			fmt.Errorf("%s: credit assessment returned unsupported outcome %q", op, assessment.Outcome),
		)
	}

	return assessment, nil
}
