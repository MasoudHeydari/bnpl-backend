package policyassignmentuc

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

const (
	trustedCustomerImportType   = "trusted_customers"
	trustedCustomerReason       = "trusted_customer"
	trustedCustomerImportSource = "csv_import"
)

// ImportSubject is one CSV row after transport-level parsing.
type ImportSubject struct {
	RowNumber int
	SubjectID string
}

// ImportTrustedInput carries one durable trusted-customer bulk import.
type ImportTrustedInput struct {
	SourceFile string
	CreatedBy  string
	Subjects   []ImportSubject
}

// ImportTrustedOutput summarizes one completed durable import.
type ImportTrustedOutput struct {
	Import       model.PolicyAssignmentImport
	SuccessCount int
	FailureCount int
}

// UseCase manages runtime policy membership imported from authoritative
// operational data. Trusted-subject CSV files are the membership source of
// truth: import does not call Identity Service or any other external verifier.
type UseCase struct {
	pool        repo.Pool
	assignments repo.PolicyAssignment
	imports     repo.PolicyAssignmentImport
	clock       clock.Clock
	logger      log.Logger
}

// New constructs the policy-assignment use case with its required collaborators.
func New(
	pool repo.Pool,
	assignments repo.PolicyAssignment,
	imports repo.PolicyAssignmentImport,
	clk clock.Clock,
	logger log.Logger,
) (*UseCase, error) {
	const op = "policyassignmentuc.New"

	if pool == nil {
		return nil, fmt.Errorf("%s: pool is required", op)
	}

	if assignments == nil {
		return nil, fmt.Errorf("%s: policy assignment repository is required", op)
	}

	if imports == nil {
		return nil, fmt.Errorf("%s: policy assignment import repository is required", op)
	}

	if clk == nil {
		return nil, fmt.Errorf("%s: clock is required", op)
	}

	if logger == nil {
		return nil, fmt.Errorf("%s: logger is required", op)
	}

	return &UseCase{
		pool:        pool,
		assignments: assignments,
		imports:     imports,
		clock:       clk,
		logger:      logger,
	}, nil
}

// ImportTrusted performs partial-success bulk admission from an authoritative
// CSV. Callers must already have validated CSV shape; this method then validates
// each subject id, creates a durable import batch, and assigns the trusted
// policy bundle per row. A persistence failure for one subject is recorded as a
// failed import item and does not roll back successful rows from the same file.
func (uc *UseCase) ImportTrusted(ctx context.Context, input ImportTrustedInput) (ImportTrustedOutput, error) {
	const op = "policyassignmentuc.UseCase.ImportTrusted"

	if strings.TrimSpace(input.SourceFile) == "" {
		return ImportTrustedOutput{}, fmt.Errorf("%s: source file is required", op)
	}

	if strings.TrimSpace(input.CreatedBy) == "" {
		return ImportTrustedOutput{}, fmt.Errorf("%s: actor is required", op)
	}

	if len(input.Subjects) == 0 {
		return ImportTrustedOutput{}, fmt.Errorf("%s: contains no subjects", op)
	}

	for _, subject := range input.Subjects {
		if subject.RowNumber <= 1 {
			return ImportTrustedOutput{}, fmt.Errorf("%s: row number must refer to a CSV data row", op)
		}

		if err := model.ValidatePolicyAssignmentSubjectID(strings.TrimSpace(subject.SubjectID)); err != nil {
			return ImportTrustedOutput{}, fmt.Errorf("%s: row %d: %w", op, subject.RowNumber, err)
		}
	}

	now := uc.clock.Now().UTC()
	var batch model.PolicyAssignmentImport
	items := make([]model.PolicyAssignmentImportItem, 0, len(input.Subjects))

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			importsQ := uc.imports.Tx(tx)
			created, err := importsQ.Create(
				ctx,
				trustedCustomerImportType,
				input.SourceFile,
				len(input.Subjects),
				strings.TrimSpace(input.CreatedBy),
				now,
			)
			if err != nil {
				return fmt.Errorf("%s: create import batch: %w", op, err)
			}

			batch = created
			for _, subject := range input.Subjects {
				item, err := importsQ.CreateItem(
					ctx,
					batch.ID,
					subject.RowNumber,
					strings.TrimSpace(subject.SubjectID),
					now,
				)
				if err != nil {
					return fmt.Errorf("%s: create import item row %d: %w", op, subject.RowNumber, err)
				}

				items = append(items, item)
			}

			return nil
		})
	})
	if err != nil {
		return ImportTrustedOutput{}, err
	}

	successCount := 0
	failureCount := 0
	for _, item := range items {
		if err := uc.assignTrustedSubject(ctx, item, input.CreatedBy); err != nil {
			if markErr := uc.markItemFailed(ctx, item.ID, "POLICY_ASSIGNMENT_ERROR", err.Error()); markErr != nil {
				return ImportTrustedOutput{}, fmt.Errorf(
					"%s: assign trusted subject: %w",
					op,
					errors.Join(err, markErr),
				)
			}

			uc.logger.Error(
				"trusted policy assignment persistence failed",
				"subject_id",
				item.SubjectID,
				"row_number",
				item.RowNumber,
				"error",
				err,
			)
			failureCount++
			continue
		}

		successCount++
	}

	status := model.CompletedPolicyAssignmentImportStatus
	if failureCount > 0 {
		status = model.CompletedWithFailuresPolicyAssignmentImportStatus
	}

	completedAt := uc.clock.Now().UTC()
	err = uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			importsQ := uc.imports.Tx(tx)
			completed, err := importsQ.Complete(
				ctx,
				batch.ID,
				status,
				successCount,
				failureCount,
				completedAt,
			)
			if err != nil {
				return fmt.Errorf("%s: complete import batch: %w", op, err)
			}

			batch = completed
			return nil
		})
	})
	if err != nil {
		return ImportTrustedOutput{}, err
	}

	return ImportTrustedOutput{
		Import:       batch,
		SuccessCount: successCount,
		FailureCount: failureCount,
	}, nil
}

// assignTrustedSubject applies every target in TrustedCustomerBundle inside one
// transaction, then marks the import item succeeded. Matching active
// assignments are left unchanged; mismatched ones are closed and replaced so
// re-imports stay idempotent.
func (uc *UseCase) assignTrustedSubject(
	ctx context.Context,
	item model.PolicyAssignmentImportItem,
	actor string,
) error {
	const op = "policyassignmentuc.UseCase.assignTrustedSubject"

	now := uc.clock.Now().UTC()
	return uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			assignmentQ := uc.assignments.Tx(tx)
			for _, target := range policy.TrustedCustomerBundle() {
				existing, err := assignmentQ.LockActive(
					ctx,
					model.CustomerPolicySubjectType,
					item.SubjectID,
					target.Family,
					now,
				)
				if err == nil {
					if existing.PolicyID == target.Ref.ID && existing.PolicyVersion == target.Ref.Version {
						continue
					}

					if _, err = assignmentQ.CloseActive(ctx, existing.ID, now, now); err != nil {
						return fmt.Errorf("%s: close active assignment: %w", op, err)
					}

				} else if !cerr.IsCode(err, cerr.CodeResourceNotFound) {
					return fmt.Errorf("%s: lock active assignment: %w", op, err)
				}

				_, err = assignmentQ.Create(
					ctx,
					model.CustomerPolicySubjectType,
					item.SubjectID,
					target.Family,
					target.Ref.ID,
					target.Ref.Version,
					trustedCustomerReason,
					trustedCustomerImportSource,
					now,
					strings.TrimSpace(actor),
					now,
					now,
				)
				if err != nil {
					return fmt.Errorf("%s: create assignment: %w", op, err)
				}
			}

			importsQ := uc.imports.Tx(tx)
			if _, err := importsQ.MarkItemSucceeded(ctx, item.ID, now); err != nil {
				return fmt.Errorf("%s: mark item succeeded: %w", op, err)
			}

			return nil
		})
	})
}

// markItemFailed records a durable per-row failure without touching other import items.
func (uc *UseCase) markItemFailed(
	ctx context.Context,
	id model.PolicyAssignmentImportItemID,
	code, message string,
) error {
	const op = "policyassignmentuc.UseCase.markItemFailed"

	now := uc.clock.Now().UTC()
	return uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			importsQ := uc.imports.Tx(tx)
			if _, err := importsQ.MarkItemFailed(
				ctx,
				id,
				strings.TrimSpace(code),
				strings.TrimSpace(message),
				now,
			); err != nil {
				return fmt.Errorf("%s: mark item failed: %w", op, err)
			}

			return nil
		})
	})
}
