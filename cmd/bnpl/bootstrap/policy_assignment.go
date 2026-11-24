package bootstrap

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/parstis/bnpl-backend/pkg/adapter/config"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/invariants"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/policyassignmentimportrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/trustedmobilerp"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/trustedmobileuc"
)

// ImportTrustedPolicyAssignments runs the operational one-shot trusted cohort
// importer from a mobile/national-ID CSV. This flow only persists immutable
// trusted membership and deliberately never calls Customer Reference Service.
// Credit assignment later verifies the imported national ID against CRS.
func ImportTrustedPolicyAssignments(sourceFile, actor string) (trustedmobileuc.ImportOutput, error) {
	const op = "bootstrap.ImportTrustedPolicyAssignments"

	customers, err := readTrustedCustomersCSV(sourceFile)
	if err != nil {
		return trustedmobileuc.ImportOutput{}, fmt.Errorf("%s: %w", op, err)
	}

	c, err := config.Load(cfgPath)
	if err != nil {
		return trustedmobileuc.ImportOutput{}, fmt.Errorf("%s: load config: %w", op, err)
	}

	// Import uses no external service, circuit breaker, or customer resolver.
	ctx := context.Background()

	logger, err := c.NewLoggerInfrastructure()
	if err != nil {
		return trustedmobileuc.ImportOutput{}, fmt.Errorf("%s: initialize logger: %w", op, err)
	}

	pool, err := c.ConnectionPool(ctx, repo.NormalRole, logger)
	if err != nil {
		return trustedmobileuc.ImportOutput{}, fmt.Errorf("%s: create database pool: %w", op, err)
	}

	defer func() { _ = pool.Close() }()

	if err = invariants.VerifyRuntime(ctx, pool); err != nil {
		return trustedmobileuc.ImportOutput{}, err
	}

	clk, err := c.NewSystemClockInfrastructure()
	if err != nil {
		return trustedmobileuc.ImportOutput{}, fmt.Errorf("%s: initialize clock: %w", op, err)
	}

	jurisdictions, err := c.NewJurisdictionCapabilities()
	if err != nil {
		return trustedmobileuc.ImportOutput{}, fmt.Errorf("%s: initialize jurisdictions: %w", op, err)
	}

	phones, err := c.NewPhoneCanonicalizer(jurisdictions)
	if err != nil {
		return trustedmobileuc.ImportOutput{}, fmt.Errorf("%s: initialize phone canonicalizer: %w", op, err)
	}

	uc, err := trustedmobileuc.New(
		pool,
		trustedmobilerp.New(),
		policyassignmentimportrp.New(),
		phones,
		clk,
		logger,
	)
	if err != nil {
		return trustedmobileuc.ImportOutput{}, fmt.Errorf("%s: initialize trusted mobile use case: %w", op, err)
	}

	return uc.Import(ctx, trustedmobileuc.ImportInput{
		SourceFile: sourceFile,
		CreatedBy:  actor,
		Customers:  customers,
	})
}

func readTrustedCustomersCSV(path string) ([]trustedmobileuc.ImportCustomer, error) {
	const op = "bootstrap.readTrustedCustomersCSV"

	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("%s: --file is required", op)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%s: open CSV: %w", op, err)
	}

	defer func() { _ = f.Close() }()

	reader := csv.NewReader(f)
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("%s: read header: %w", op, err)
	}

	if len(header) != 2 || strings.TrimSpace(header[0]) != "mobile_e164" || strings.TrimSpace(header[1]) != "national_id" {
		return nil, fmt.Errorf("%s: header must be exactly: mobile_e164,national_id", op)
	}

	rows := make([]trustedmobileuc.ImportCustomer, 0)
	for rowNumber := 2; ; rowNumber++ {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("%s: read row %d: %w", op, rowNumber, err)
		}

		if len(record) != 2 {
			return nil, fmt.Errorf("%s: row %d must contain exactly 2 columns", op, rowNumber)
		}

		// Preserve empty fields for durable per-row invalid input outcomes.
		rows = append(rows, trustedmobileuc.ImportCustomer{
			RowNumber:  rowNumber,
			MobileE164: record[0],
			NationalID: record[1],
		})
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("%s: CSV contains no data rows", op)
	}

	return rows, nil
}
