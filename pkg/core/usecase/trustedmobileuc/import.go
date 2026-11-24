// Package trustedmobileuc imports raw trusted mobile/national-ID pairs.
// Import deliberately does not call Customer Reference Service or assign credit.
package trustedmobileuc

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/parstis/bnpl-backend/pkg/core/canonicalizer/phonenumber"
	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var nationalIDPattern = regexp.MustCompile(`^[0-9]{10}$`)

const (
	importType               = "trusted_customers"
	outcomeInvalidMobile     = "INVALID_MOBILE"
	outcomeInvalidNationalID = "INVALID_NATIONAL_ID"
	outcomeDuplicate         = "DUPLICATE"
	outcomeConflict          = "NATIONAL_ID_CONFLICT"
	outcomeDatabaseError     = "DATABASE_ERROR"
)

type ImportCustomer struct {
	RowNumber  int
	MobileE164 string
	NationalID string
}

type ImportInput struct {
	SourceFile string
	CreatedBy  string
	Customers  []ImportCustomer
}

type ImportOutput struct {
	Import            model.PolicyAssignmentImport
	SuccessCount      int
	FailureCount      int
	Added             int
	AlreadyExists     int
	InvalidMobile     int
	InvalidNationalID int
	Duplicates        int
	Conflicts         int
	DatabaseErrors    int
}

func (o ImportOutput) FormatSummary() string {
	return fmt.Sprintf(`Trusted Mobile Import Summary
--------------------------------
Total rows:             %4d
Added:                  %4d
Already exists:         %4d
Invalid mobile:         %4d
Invalid national ID:    %4d
Duplicates:             %4d
National ID conflicts:  %4d
Database errors:        %4d
--------------------------------
Succeeded:              %4d
Failed:                 %4d
`, o.SuccessCount+o.FailureCount, o.Added, o.AlreadyExists, o.InvalidMobile, o.InvalidNationalID, o.Duplicates, o.Conflicts, o.DatabaseErrors, o.SuccessCount, o.FailureCount)
}

type UseCase struct {
	pool     repo.Pool
	registry repo.TrustedMobile
	imports  repo.PolicyAssignmentImport
	phones   phonenumber.Canonicalizer
	clock    clock.Clock
	logger   log.Logger
}

func New(pool repo.Pool, registry repo.TrustedMobile, imports repo.PolicyAssignmentImport, phones phonenumber.Canonicalizer, clk clock.Clock, logger log.Logger) (*UseCase, error) {
	if pool == nil || registry == nil || imports == nil || phones == nil || clk == nil || logger == nil {
		return nil, errors.New("trustedmobileuc.New: all dependencies are required")
	}
	return &UseCase{pool: pool, registry: registry, imports: imports, phones: phones, clock: clk, logger: logger}, nil
}

// Import records independent rows with a durable result for every CSV row.
// Re-importing an identical pair succeeds without mutation; a different
// national ID for an existing phone is a conflict.
func (uc *UseCase) Import(ctx context.Context, input ImportInput) (ImportOutput, error) {
	const op = "trustedmobileuc.UseCase.Import"
	if strings.TrimSpace(input.SourceFile) == "" || strings.TrimSpace(input.CreatedBy) == "" || len(input.Customers) == 0 {
		return ImportOutput{}, fmt.Errorf("%s: source, actor and at least one row are required", op)
	}
	for _, row := range input.Customers {
		if row.RowNumber <= 1 {
			return ImportOutput{}, fmt.Errorf("%s: invalid CSV row number", op)
		}
	}

	now := uc.clock.Now().UTC()
	var batch model.PolicyAssignmentImport
	if err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			var err error
			batch, err = uc.imports.Tx(tx).Create(ctx, importType, input.SourceFile, len(input.Customers), strings.TrimSpace(input.CreatedBy), now)
			return err
		})
	}); err != nil {
		return ImportOutput{}, fmt.Errorf("%s: create batch: %w", op, err)
	}

	out := ImportOutput{Import: batch}
	seen := make(map[string]string, len(input.Customers))
	for _, row := range input.Customers {
		mobile, err := uc.phones.Canonicalize(model.IRJurisdiction, strings.TrimSpace(row.MobileE164))
		var code string
		switch {
		case err != nil:
			out.InvalidMobile++
			code = outcomeInvalidMobile
		case !nationalIDPattern.MatchString(strings.TrimSpace(row.NationalID)):
			out.InvalidNationalID++
			code = outcomeInvalidNationalID
		default:
			nationalID := strings.TrimSpace(row.NationalID)
			if prior, exists := seen[mobile]; exists {
				if prior == nationalID {
					out.Duplicates++
					code = outcomeDuplicate
				} else {
					out.Conflicts++
					code = outcomeConflict
				}
			} else {
				seen[mobile] = nationalID
				var added bool
				added, err = uc.insertRow(ctx, batch.ID, row.RowNumber, mobile, strings.TrimSpace(row.NationalID), strings.TrimSpace(input.CreatedBy))
				switch {
				case err == nil:
					if added {
						out.Added++
					} else {
						out.AlreadyExists++
					}
				case errors.Is(err, repo.ErrTrustedMobileNationalIDConflict):
					out.Conflicts++
					code = outcomeConflict
				default:
					out.DatabaseErrors++
					code = outcomeDatabaseError
				}
			}
		}
		if code != "" {
			if err := uc.failRow(ctx, batch.ID, row.RowNumber, strings.TrimSpace(row.NationalID), code); err != nil {
				return ImportOutput{}, fmt.Errorf("%s: audit row %d: %w", op, row.RowNumber, err)
			}
			uc.logger.With("row_number", row.RowNumber, "reason", code).Warn(log.Msg("trusted_mobile_import_row_failed"))
		}
	}
	out.SuccessCount = out.Added + out.AlreadyExists
	out.FailureCount = out.InvalidMobile + out.InvalidNationalID + out.Duplicates + out.Conflicts + out.DatabaseErrors
	status := model.CompletedPolicyAssignmentImportStatus
	if out.FailureCount > 0 {
		status = model.CompletedWithFailuresPolicyAssignmentImportStatus
	}
	if err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			var err error
			batch, err = uc.imports.Tx(tx).Complete(ctx, batch.ID, status, out.SuccessCount, out.FailureCount, uc.clock.Now().UTC())
			return err
		})
	}); err != nil {
		return ImportOutput{}, fmt.Errorf("%s: complete batch: %w", op, err)
	}
	out.Import = batch
	return out, nil
}

func (uc *UseCase) insertRow(ctx context.Context, importID model.PolicyAssignmentImportID, rowNumber int, mobile, nationalID, actor string) (bool, error) {
	now := uc.clock.Now().UTC()
	var added bool
	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			q := uc.imports.Tx(tx)
			item, err := q.CreateItem(ctx, importID, rowNumber, &nationalID, nil, now)
			if err != nil {
				return err
			}
			added, err = uc.registry.Tx(tx).InsertIfAbsent(ctx, mobile, nationalID, actor, now)
			if err != nil {
				return err
			}
			_, err = q.MarkItemSucceeded(ctx, item.ID, now)
			return err
		})
	})
	return added, err
}

func (uc *UseCase) failRow(ctx context.Context, importID model.PolicyAssignmentImportID, rowNumber int, nationalID, code string) error {
	now := uc.clock.Now().UTC()
	var national *string
	if nationalIDPattern.MatchString(nationalID) {
		national = &nationalID
	}
	return uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			q := uc.imports.Tx(tx)
			item, err := q.CreateItem(ctx, importID, rowNumber, national, nil, now)
			if err != nil {
				return err
			}
			_, err = q.MarkItemFailed(ctx, item.ID, code, strings.ToLower(strings.ReplaceAll(code, "_", " ")), now)
			return err
		})
	})
}
