package policyassignmentuc

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/parstis/bnpl-backend/pkg/core/canonicalizer/phonenumber"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/clock"
	extcustomer "github.com/parstis/bnpl-backend/pkg/core/external/customer"
	"github.com/parstis/bnpl-backend/pkg/core/fingerprint"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

const (
	trustedCustomerImportType   = "trusted_customers"
	trustedCustomerReason       = "trusted_customer"
	trustedCustomerImportSource = "csv_import"

	outcomeAdded         = "added"
	outcomeAlreadyExists = "already_exists"
	outcomeNotFound      = "not_found"
	outcomeInvalidStatus = "invalid_status"
	outcomeInvalidMobile = "invalid_mobile"
	outcomeDuplicate     = "duplicate"
	outcomeResolveError  = "resolve_error"
	outcomeDatabaseError = "database_error"

	errorCodeInvalidMobile = "INVALID_MOBILE"
	errorCodeNotFound      = "NOT_FOUND"
	errorCodeInvalidStatus = "INVALID_STATUS"
	errorCodeDuplicate     = "DUPLICATE"
	errorCodeResolveError  = "RESOLVE_ERROR"
	errorCodeDatabaseError = "DATABASE_ERROR"
)

// ImportCustomer is one trusted-customer CSV data row before resolution.
type ImportCustomer struct {
	// RowNumber is the 1-based CSV line number (header is row 1).
	RowNumber int
	// MobileE164 is the raw CSV mobile value (may be local 09… or +989…).
	MobileE164 string
}

// ImportTrustedInput carries one durable trusted-customer bulk import.
type ImportTrustedInput struct {
	SourceFile string
	CreatedBy  string
	Customers  []ImportCustomer
}

// ImportTrustedOutput summarizes one completed durable import.
type ImportTrustedOutput struct {
	Import       model.PolicyAssignmentImport
	SuccessCount int
	FailureCount int

	Added                 int
	AlreadyExists         int
	NotFound              int
	InvalidCustomerStatus int
	InvalidMobile         int
	Duplicates            int
	CustomerServiceErrors int
	DatabaseErrors        int
}

// UseCase manages runtime policy membership imported from operational CSVs.
//
// Customer Reference Service is authoritative for mobile → ExternalCustomerID
// resolution. Network calls never run inside a database transaction.
type UseCase struct {
	pool         repo.Pool
	assignments  repo.PolicyAssignment
	imports      repo.PolicyAssignmentImport
	customers    extcustomer.Resolver
	phones       phonenumber.Canonicalizer
	mobileHasher fingerprint.Hasher
	clock        clock.Clock
	logger       log.Logger
}

// New constructs the policy-assignment use case with its required collaborators.
func New(
	pool repo.Pool,
	assignments repo.PolicyAssignment,
	imports repo.PolicyAssignmentImport,
	customers extcustomer.Resolver,
	phones phonenumber.Canonicalizer,
	mobileHasher fingerprint.Hasher,
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

	if customers == nil {
		return nil, fmt.Errorf("%s: customer resolver is required", op)
	}

	if phones == nil {
		return nil, fmt.Errorf("%s: phone canonicalizer is required", op)
	}

	if mobileHasher == nil {
		return nil, fmt.Errorf("%s: mobile hasher is required", op)
	}

	if clk == nil {
		return nil, fmt.Errorf("%s: clock is required", op)
	}

	if logger == nil {
		return nil, fmt.Errorf("%s: logger is required", op)
	}

	return &UseCase{
		pool:         pool,
		assignments:  assignments,
		imports:      imports,
		customers:    customers,
		phones:       phones,
		mobileHasher: mobileHasher,
		clock:        clk,
		logger:       logger,
	}, nil
}

// ImportTrusted performs partial-success bulk admission from a mobile CSV.
//
// For each row the use case validates/canonicalizes the mobile, resolves the
// customer outside any DB transaction, and only then opens a transaction to
// apply policy.TrustedCustomerBundle() for ACTIVE customers. Row-level failures
// are logged and recorded as durable import items without stopping the file.
func (uc *UseCase) ImportTrusted(ctx context.Context, input ImportTrustedInput) (ImportTrustedOutput, error) {
	const op = "policyassignmentuc.UseCase.ImportTrusted"

	if strings.TrimSpace(input.SourceFile) == "" {
		return ImportTrustedOutput{}, fmt.Errorf("%s: source file is required", op)
	}

	if strings.TrimSpace(input.CreatedBy) == "" {
		return ImportTrustedOutput{}, fmt.Errorf("%s: actor is required", op)
	}

	if len(input.Customers) == 0 {
		return ImportTrustedOutput{}, fmt.Errorf("%s: contains no customers", op)
	}

	for _, customer := range input.Customers {
		if customer.RowNumber <= 1 {
			return ImportTrustedOutput{}, fmt.Errorf("%s: row number must refer to a CSV data row", op)
		}
	}

	now := uc.clock.Now().UTC()
	var batch model.PolicyAssignmentImport

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			importsQ := uc.imports.Tx(tx)
			created, err := importsQ.Create(
				ctx,
				trustedCustomerImportType,
				input.SourceFile,
				len(input.Customers),
				strings.TrimSpace(input.CreatedBy),
				now,
			)
			if err != nil {
				return fmt.Errorf("%s: create import batch: %w", op, err)
			}

			batch = created
			return nil
		})
	})
	if err != nil {
		return ImportTrustedOutput{}, err
	}

	summary := ImportTrustedOutput{Import: batch}
	seenMobileHash := make(map[string]struct{}, len(input.Customers))
	seenExternalID := make(map[model.ExternalCustomerID]struct{}, len(input.Customers))

	for _, customer := range input.Customers {
		outcome := uc.processTrustedRow(
			ctx,
			batch.ID,
			customer,
			strings.TrimSpace(input.CreatedBy),
			seenMobileHash,
			seenExternalID,
		)
		summary.recordOutcome(outcome)
	}

	summary.SuccessCount = summary.Added + summary.AlreadyExists
	summary.FailureCount = summary.NotFound +
		summary.InvalidCustomerStatus +
		summary.InvalidMobile +
		summary.Duplicates +
		summary.CustomerServiceErrors +
		summary.DatabaseErrors

	status := model.CompletedPolicyAssignmentImportStatus
	if summary.FailureCount > 0 {
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
				summary.SuccessCount,
				summary.FailureCount,
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

	summary.Import = batch
	return summary, nil
}

func (o *ImportTrustedOutput) recordOutcome(outcome string) {
	switch outcome {
	case outcomeAdded:
		o.Added++

	case outcomeAlreadyExists:
		o.AlreadyExists++

	case outcomeNotFound:
		o.NotFound++

	case outcomeInvalidStatus:
		o.InvalidCustomerStatus++

	case outcomeInvalidMobile:
		o.InvalidMobile++

	case outcomeDuplicate:
		o.Duplicates++

	case outcomeResolveError:
		o.CustomerServiceErrors++

	case outcomeDatabaseError:
		o.DatabaseErrors++
	}
}

// FormatSummary returns the human-readable CLI summary for stdout.
func (o ImportTrustedOutput) FormatSummary() string {
	total := o.Added +
		o.AlreadyExists +
		o.NotFound +
		o.InvalidCustomerStatus +
		o.InvalidMobile +
		o.Duplicates +
		o.CustomerServiceErrors +
		o.DatabaseErrors

	return fmt.Sprintf(
		`Trusted Customers Import Summary
--------------------------------
Total rows:                 %4d
Added:                      %4d
Already exists:             %4d
Not found:                  %4d
Invalid customer status:    %4d
Invalid mobile:             %4d
Duplicates:                 %4d
Customer Service errors:    %4d
Database errors:            %4d
--------------------------------
Processed:                  %4d
`,
		total,
		o.Added,
		o.AlreadyExists,
		o.NotFound,
		o.InvalidCustomerStatus,
		o.InvalidMobile,
		o.Duplicates,
		o.CustomerServiceErrors,
		o.DatabaseErrors,
		total,
	)
}

func (uc *UseCase) processTrustedRow(
	ctx context.Context,
	importID model.PolicyAssignmentImportID,
	customer ImportCustomer,
	actor string,
	seenMobileHash map[string]struct{},
	seenExternalID map[model.ExternalCustomerID]struct{},
) string {
	rawMobile := strings.TrimSpace(customer.MobileE164)
	canonical, err := uc.phones.Canonicalize(model.IRJurisdiction, rawMobile)
	if err != nil {
		uc.logRow(
			"trusted_customer_skipped",
			outcomeInvalidMobile,
			"invalid_mobile",
			customer.RowNumber,
			"",
			model.ExternalCustomerID{},
			"",
			"",
			err,
		)
		_ = uc.recordFailedItem(
			ctx,
			importID,
			customer.RowNumber,
			nil,
			nil,
			errorCodeInvalidMobile,
			"invalid mobile number",
		)
		return outcomeInvalidMobile
	}

	hashValue, err := uc.mobileHasher.Hash(canonical)
	if err != nil {
		uc.logRow(
			"trusted_customer_failed",
			outcomeDatabaseError,
			"mobile_hash_error",
			customer.RowNumber,
			"",
			model.ExternalCustomerID{},
			"",
			"",
			err,
		)
		_ = uc.recordFailedItem(
			ctx,
			importID,
			customer.RowNumber,
			nil,
			nil,
			errorCodeDatabaseError,
			"mobile hash failed",
		)
		return outcomeDatabaseError
	}

	mobileHash := hashValue.String()
	if _, exists := seenMobileHash[mobileHash]; exists {
		uc.logRow(
			"trusted_customer_skipped",
			outcomeDuplicate,
			"duplicate_mobile",
			customer.RowNumber,
			mobileHash,
			model.ExternalCustomerID{},
			"",
			"",
			nil,
		)
		_ = uc.recordFailedItem(
			ctx,
			importID,
			customer.RowNumber,
			nil,
			&mobileHash,
			errorCodeDuplicate,
			"duplicate mobile in CSV",
		)
		return outcomeDuplicate
	}

	seenMobileHash[mobileHash] = struct{}{}

	resolved, err := uc.customers.ResolveByMobile(ctx, extcustomer.ResolveByMobileInput{
		MobileE164: canonical,
	})
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			uc.logRow(
				"trusted_customer_skipped",
				outcomeNotFound,
				"customer_not_found",
				customer.RowNumber,
				mobileHash,
				model.ExternalCustomerID{},
				"",
				"",
				err,
			)
			_ = uc.recordFailedItem(
				ctx,
				importID,
				customer.RowNumber,
				nil,
				&mobileHash,
				errorCodeNotFound,
				"customer not found",
			)
			return outcomeNotFound
		}

		uc.logRow(
			"trusted_customer_skipped",
			outcomeResolveError,
			"customer_resolve_error",
			customer.RowNumber,
			mobileHash,
			model.ExternalCustomerID{},
			"",
			"",
			err,
		)
		_ = uc.recordFailedItem(
			ctx,
			importID,
			customer.RowNumber,
			nil,
			&mobileHash,
			errorCodeResolveError,
			"customer resolve failed",
		)
		return outcomeResolveError
	}

	subjectID := resolved.ExternalCustomerID.String()
	if _, exists := seenExternalID[resolved.ExternalCustomerID]; exists {
		uc.logRow(
			"trusted_customer_skipped",
			outcomeDuplicate,
			"duplicate_external_customer_id",
			customer.RowNumber,
			mobileHash,
			resolved.ExternalCustomerID,
			"",
			"",
			nil,
		)
		_ = uc.recordFailedItem(
			ctx,
			importID,
			customer.RowNumber,
			&subjectID,
			&mobileHash,
			errorCodeDuplicate,
			"duplicate external customer id in CSV",
		)
		return outcomeDuplicate
	}

	if resolved.Status != extcustomer.ActiveStatus {
		uc.logger.Info(
			"event=trusted_customer_skipped",
			"reason", "invalid_customer_status",
			"expected_status", string(extcustomer.ActiveStatus),
			"actual_status", string(resolved.Status),
			"row_number", customer.RowNumber,
			"mobile_hash", mobileHash,
			"external_customer_id", subjectID,
			"outcome", outcomeInvalidStatus,
		)
		_ = uc.recordFailedItem(
			ctx,
			importID,
			customer.RowNumber,
			&subjectID,
			&mobileHash,
			errorCodeInvalidStatus,
			fmt.Sprintf("customer status %s; expected %s", resolved.Status, extcustomer.ActiveStatus),
		)
		return outcomeInvalidStatus
	}

	outcome, err := uc.assignTrustedCustomer(
		ctx,
		importID,
		customer.RowNumber,
		resolved.ExternalCustomerID,
		mobileHash,
		actor,
	)
	if err != nil {
		uc.logRow(
			"trusted_customer_failed",
			outcomeDatabaseError,
			"policy_assignment_error",
			customer.RowNumber,
			mobileHash,
			resolved.ExternalCustomerID,
			"",
			"",
			err,
		)
		_ = uc.recordFailedItem(
			ctx,
			importID,
			customer.RowNumber,
			&subjectID,
			&mobileHash,
			errorCodeDatabaseError,
			"policy assignment persistence failed",
		)
		return outcomeDatabaseError
	}

	seenExternalID[resolved.ExternalCustomerID] = struct{}{}
	uc.logRow(
		"trusted_customer_processed",
		outcome,
		outcome,
		customer.RowNumber,
		mobileHash,
		resolved.ExternalCustomerID,
		string(extcustomer.ActiveStatus),
		string(resolved.Status),
		nil,
	)
	return outcome
}

func (uc *UseCase) logRow(
	event string,
	outcome string,
	reason string,
	rowNumber int,
	mobileHash string,
	externalCustomerID model.ExternalCustomerID,
	expectedStatus string,
	actualStatus string,
	err error,
) {
	args := []any{
		"event", event,
		"outcome", outcome,
		"reason", reason,
		"row_number", rowNumber,
	}
	if mobileHash != "" {
		args = append(args, "mobile_hash", mobileHash)
	}

	if !externalCustomerID.IsZero() {
		args = append(args, "external_customer_id", externalCustomerID.String())
	}

	if expectedStatus != "" {
		args = append(args, "expected_status", expectedStatus)
	}

	if actualStatus != "" {
		args = append(args, "actual_status", actualStatus)
	}

	if err != nil {
		args = append(args, "error", err.Error())
		uc.logger.Warn(event, args...)
		return
	}

	uc.logger.Info(event, args...)
}

// assignTrustedCustomer applies every target in TrustedCustomerBundle inside one
// transaction together with the durable succeeded import item. Matching active
// assignments are left unchanged; mismatched ones are closed and replaced so
// re-imports stay idempotent and additive.
func (uc *UseCase) assignTrustedCustomer(
	ctx context.Context,
	importID model.PolicyAssignmentImportID,
	rowNumber int,
	externalCustomerID model.ExternalCustomerID,
	mobileHash string,
	actor string,
) (string, error) {
	const op = "policyassignmentuc.UseCase.assignTrustedCustomer"

	now := uc.clock.Now().UTC()
	subjectID := externalCustomerID.String()
	outcome := outcomeAlreadyExists

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			importsQ := uc.imports.Tx(tx)
			item, err := importsQ.CreateItem(
				ctx,
				importID,
				rowNumber,
				&subjectID,
				&mobileHash,
				now,
			)
			if err != nil {
				return fmt.Errorf("%s: create import item: %w", op, err)
			}

			assignmentQ := uc.assignments.Tx(tx)
			for _, target := range policy.TrustedCustomerBundle() {
				existing, err := assignmentQ.LockActive(
					ctx,
					model.CustomerPolicySubjectType,
					subjectID,
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

					outcome = outcomeAdded
				} else if !cerr.IsCode(err, cerr.CodeResourceNotFound) {
					return fmt.Errorf("%s: lock active assignment: %w", op, err)
				} else {
					outcome = outcomeAdded
				}

				_, err = assignmentQ.Create(
					ctx,
					model.CustomerPolicySubjectType,
					subjectID,
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

			if _, err := importsQ.MarkItemSucceeded(ctx, item.ID, now); err != nil {
				return fmt.Errorf("%s: mark item succeeded: %w", op, err)
			}

			return nil
		})
	})
	if err != nil {
		return "", err
	}

	return outcome, nil
}

func (uc *UseCase) recordFailedItem(
	ctx context.Context,
	importID model.PolicyAssignmentImportID,
	rowNumber int,
	subjectID *string,
	mobileHash *string,
	code, message string,
) error {
	const op = "policyassignmentuc.UseCase.recordFailedItem"

	now := uc.clock.Now().UTC()
	return uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			importsQ := uc.imports.Tx(tx)
			item, err := importsQ.CreateItem(
				ctx,
				importID,
				rowNumber,
				subjectID,
				mobileHash,
				now,
			)
			if err != nil {
				return fmt.Errorf("%s: create import item: %w", op, err)
			}

			if _, err := importsQ.MarkItemFailed(
				ctx,
				item.ID,
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

// ErrNoopCustomerResolver is returned by bootstrap when the configured customer
// provider is noop. Trusted import must resolve mobiles authoritatively.
var ErrNoopCustomerResolver = errors.New("customer resolver must not be noop for trusted import")
