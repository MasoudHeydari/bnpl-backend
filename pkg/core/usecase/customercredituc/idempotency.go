package customercredituc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

const (
	customerCreditLimitFingerprintVersion = "customer-credit-limit:v2:"
	maxDecisionIdempotencyKeyLength       = 256
)

func isPlausibleIdempotencyCreateConflict(err error) bool {
	if err == nil {
		return false
	}

	return cerr.IsCode(err, cerr.CodeValidationFailed) || cerr.IsCode(err, cerr.CodeConflict)
}

// initialLimitIdempotencyKey scopes idempotency to one governed assessment
// cycle. Approved cycles advance with each persisted LIMIT_SET credit journal
// entry. Denied assessments use cycle zero because they do not mutate a credit
// account.
func initialLimitIdempotencyKey(
	customerID model.CustomerID,
	assessment InitialLimitAssessmentResult,
	assignmentCycle int64,
) string {
	raw := strings.Join([]string{
		"initial-credit-limit",
		customerID.String(),
		assessment.PolicyID,
		assessment.PolicyVersion,
		string(assessment.Outcome),
		primaryReasonCode(assessment),
		strconv.FormatInt(assignmentCycle, 10),
	}, ":")

	if len(raw) <= maxDecisionIdempotencyKeyLength {
		return raw
	}

	digest := sha256.Sum256([]byte(raw))
	return "initial-credit-limit:sha256:" + hex.EncodeToString(digest[:])
}

func assignmentCycleFor(
	ctx context.Context,
	accountQ repo.CreditAccountQueryer,
	customerID model.CustomerID,
	assessment InitialLimitAssessmentResult,
) (int64, error) {
	if assessment.Outcome != model.LimitSetDecisionOutcome {
		return 0, nil
	}

	return accountQ.CountLimitSetTransactions(ctx, customerID, assessment.Currency)
}

func primaryReasonCode(assessment InitialLimitAssessmentResult) string {
	if len(assessment.ReasonCodes) == 0 {
		return ""
	}

	return assessment.ReasonCodes[0]
}

func resultFromResolvedOutput(output AssignInitialLimitOutput) AssignInitialLimitResult {
	if output.CreditLimit != nil {
		return AssignInitialLimitResultAlreadyAssigned
	}

	if output.Decision.Outcome == model.DeniedDecisionOutcome {
		return AssignInitialLimitResultDenied
	}

	return AssignInitialLimitResultAlreadyAssigned
}

func (uc *UseCase) initialLimitFingerprint(
	customerID model.CustomerID,
	assessment InitialLimitAssessmentResult,
	assignmentCycle int64,
) (string, error) {
	const op = "customercredituc.UseCase.initialLimitFingerprint"

	reasonCodes := append([]string(nil), assessment.ReasonCodes...)
	slices.Sort(reasonCodes)

	raw := strings.Join([]string{
		customerID.String(),
		assessment.PolicyID,
		assessment.PolicyVersion,
		string(assessment.Outcome),
		fmt.Sprintf("%d", assessment.AmountMinor),
		string(assessment.Currency),
		assessment.ValidFor.String(),
		strings.Join(reasonCodes, ","),
		strconv.FormatInt(assignmentCycle, 10),
	}, "|")

	value, err := uc.fingerprintHasher.Hash(customerCreditLimitFingerprintVersion + raw)
	if err != nil {
		return "", fmt.Errorf("%s: hash fingerprint: %w", op, err)
	}

	if err := value.Validate(); err != nil {
		return "", fmt.Errorf("%s: validate fingerprint: %w", op, err)
	}

	return value.String(), nil
}

func (uc *UseCase) resolveIdempotentAssignment(
	ctx context.Context,
	decisionQ repo.DecisionTxQueryer,
	accountQ repo.CreditAccountTxQueryer,
	idempotencyKey string,
	fingerprint string,
) (AssignInitialLimitOutput, bool, error) {
	const op = "customercredituc.UseCase.resolveIdempotentAssignment"

	existing, err := decisionQ.GetByIdempotencyKey(ctx, idempotencyKey)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return AssignInitialLimitOutput{}, false, nil
		}

		return AssignInitialLimitOutput{}, false, err
	}

	if existing.IdempotencyFingerprint != fingerprint {
		return AssignInitialLimitOutput{}, true, idempotencyConflictError(
			fmt.Errorf("%s: idempotency key reused with different initial credit limit inputs", op),
		)
	}

	output, err := uc.outputFromDecision(ctx, accountQ, existing)
	return output, true, err
}

func (uc *UseCase) resolveAssignmentAfterConflict(
	ctx context.Context,
	customerID model.CustomerID,
	assessment InitialLimitAssessmentResult,
	now time.Time,
) (AssignInitialLimitOutput, error) {
	var output AssignInitialLimitOutput
	const op = "customercredituc.UseCase.resolveAssignmentAfterConflict"

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			decisionQ := uc.decision.Tx(tx)
			accountQ := uc.creditAccount.Tx(tx)
			profileQ := uc.customerProfile.Tx(tx)

			// Serializes concurrent assessment persistence for one customer. The
			// external assessment happens outside this transaction; only durable
			// state resolution is serialized here.
			if _, err := profileQ.LockByID(ctx, customerID); err != nil {
				return err
			}

			if err := accountQ.ExpireDueByCustomerIDAndCurrency(ctx, customerID, assessment.Currency, now); err != nil {
				return err
			}

			if account, err := accountQ.LockByCustomerIDAndCurrency(ctx, customerID, assessment.Currency); err == nil && account.IsUsableAt(now) {
				resolved, loadErr := uc.outputFromExistingCreditAccount(ctx, decisionQ, account)
				if loadErr != nil {
					return loadErr
				}

				output = resolved
				return nil
			} else if err != nil && !cerr.IsCode(err, cerr.CodeResourceNotFound) {
				return err
			}

			cycle, err := assignmentCycleFor(ctx, accountQ, customerID, assessment)
			if err != nil {
				return err
			}

			key := initialLimitIdempotencyKey(customerID, assessment, cycle)
			fingerprint, err := uc.initialLimitFingerprint(customerID, assessment, cycle)
			if err != nil {
				return err
			}

			resolved, found, err := uc.resolveIdempotentAssignment(ctx, decisionQ, accountQ, key, fingerprint)
			if err != nil {
				return err
			}

			if found {
				output = resolved
				return nil
			}

			return initialLimitAssignmentConflictError(
				fmt.Errorf("%s: initial credit account assignment conflict", op),
			)
		})
	})
	if err != nil {
		return AssignInitialLimitOutput{}, err
	}

	return output, nil
}

func (uc *UseCase) outputFromDecision(
	ctx context.Context,
	accountQ repo.CreditAccountTxQueryer,
	decision model.Decision,
) (AssignInitialLimitOutput, error) {
	output := AssignInitialLimitOutput{Decision: decision}
	if decision.Outcome != model.LimitSetDecisionOutcome {
		return output, nil
	}

	account, err := accountQ.GetByLastLimitDecisionID(ctx, decision.ID)
	if err != nil {
		return AssignInitialLimitOutput{}, err
	}

	output.CreditLimit = &account
	return output, nil
}

func (uc *UseCase) outputFromExistingCreditAccount(
	ctx context.Context,
	decisionQ repo.DecisionTxQueryer,
	account model.CreditAccount,
) (AssignInitialLimitOutput, error) {
	if account.LastLimitDecisionID == nil || account.LastLimitDecisionID.IsZero() {
		return AssignInitialLimitOutput{}, cerr.InternalError(fmt.Errorf("credit account has no governing limit decision"))
	}

	decision, err := decisionQ.GetByID(ctx, *account.LastLimitDecisionID)
	if err != nil {
		return AssignInitialLimitOutput{}, err
	}

	return AssignInitialLimitOutput{Decision: decision, CreditLimit: &account}, nil
}
