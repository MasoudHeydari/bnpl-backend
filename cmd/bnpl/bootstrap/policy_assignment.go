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
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/policyassignmentrp"
	customerext "github.com/parstis/bnpl-backend/pkg/adapter/external/customer"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/policyassignmentuc"
)

// ImportTrustedPolicyAssignments runs the operational one-shot trusted cohort
// importer from a mobile CSV. Each row is resolved through Customer Reference
// Service outside any database transaction; only ACTIVE customers receive the
// trusted policy bundle. The command does not start HTTP listeners, messaging
// consumers, or background workers.
func ImportTrustedPolicyAssignments(sourceFile, actor string) (policyassignmentuc.ImportTrustedOutput, error) {
	const op = "bootstrap.ImportTrustedPolicyAssignments"

	customers, err := readTrustedCustomersCSV(sourceFile)
	if err != nil {
		return policyassignmentuc.ImportTrustedOutput{}, fmt.Errorf("%s: %w", op, err)
	}

	c, err := config.Load(cfgPath)
	if err != nil {
		return policyassignmentuc.ImportTrustedOutput{}, fmt.Errorf("%s: load config: %w", op, err)
	}

	if customerext.Provider(c.External.Customer.Provider) == customerext.NoopProvider {
		return policyassignmentuc.ImportTrustedOutput{}, fmt.Errorf(
			"%s: %w",
			op,
			policyassignmentuc.ErrNoopCustomerResolver,
		)
	}

	traceProvider, err := c.NewTraceInfrastructure()
	if err != nil {
		return policyassignmentuc.ImportTrustedOutput{}, fmt.Errorf("%s: initialize trace provider: %w", op, err)
	}

	// CLI has no Gin TraceID middleware; seed identifiers before outbound CRS gRPC.
	ctx := trace.EnsureIdentifiers(context.Background(), traceProvider)

	logger, err := c.NewLoggerInfrastructure()
	if err != nil {
		return policyassignmentuc.ImportTrustedOutput{}, fmt.Errorf("%s: initialize logger: %w", op, err)
	}

	pool, err := c.ConnectionPool(ctx, repo.NormalRole, logger)
	if err != nil {
		return policyassignmentuc.ImportTrustedOutput{}, fmt.Errorf("%s: create database pool: %w", op, err)
	}

	defer func() { _ = pool.Close() }()

	if err = invariants.VerifyRuntime(ctx, pool); err != nil {
		return policyassignmentuc.ImportTrustedOutput{}, err
	}

	clk, err := c.NewSystemClockInfrastructure()
	if err != nil {
		return policyassignmentuc.ImportTrustedOutput{}, fmt.Errorf("%s: initialize clock: %w", op, err)
	}

	breaker, err := c.NewCircuitBreakerInfrastructure()
	if err != nil {
		return policyassignmentuc.ImportTrustedOutput{}, fmt.Errorf("%s: initialize circuit breaker: %w", op, err)
	}

	customerResolver, customerCloser, err := c.NewExternalCustomerClient(breaker)
	if err != nil {
		return policyassignmentuc.ImportTrustedOutput{}, fmt.Errorf("%s: initialize customer resolver: %w", op, err)
	}

	if customerCloser != nil {
		defer func() { _ = customerCloser.Close() }()
	}

	jurisdictions, err := c.NewJurisdictionCapabilities()
	if err != nil {
		return policyassignmentuc.ImportTrustedOutput{}, fmt.Errorf("%s: initialize jurisdictions: %w", op, err)
	}

	phones, err := c.NewPhoneCanonicalizer(jurisdictions)
	if err != nil {
		return policyassignmentuc.ImportTrustedOutput{}, fmt.Errorf("%s: initialize phone canonicalizer: %w", op, err)
	}

	mobileHasher, err := c.Usecases.PolicyAssignment.NewMobileHasher()
	if err != nil {
		return policyassignmentuc.ImportTrustedOutput{}, fmt.Errorf("%s: initialize mobile hasher: %w", op, err)
	}

	uc, err := policyassignmentuc.New(
		pool,
		policyassignmentrp.New(),
		policyassignmentimportrp.New(),
		customerResolver,
		phones,
		mobileHasher,
		clk,
		logger,
	)
	if err != nil {
		return policyassignmentuc.ImportTrustedOutput{}, fmt.Errorf("%s: initialize policy assignment use case: %w", op, err)
	}

	return uc.ImportTrusted(ctx, policyassignmentuc.ImportTrustedInput{
		SourceFile: sourceFile,
		CreatedBy:  actor,
		Customers:  customers,
	})
}

func readTrustedCustomersCSV(path string) ([]policyassignmentuc.ImportCustomer, error) {
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

	if len(header) != 1 || strings.TrimSpace(header[0]) != "mobile_e164" {
		return nil, fmt.Errorf("%s: header must be exactly: mobile_e164", op)
	}

	rows := make([]policyassignmentuc.ImportCustomer, 0)
	for rowNumber := 2; ; rowNumber++ {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("%s: read row %d: %w", op, rowNumber, err)
		}

		mobile := ""
		if len(record) > 0 {
			mobile = record[0]
		}

		// Keep empty / multi-column rows so the use case can record row-level
		// invalid_mobile outcomes instead of aborting the whole file.
		rows = append(rows, policyassignmentuc.ImportCustomer{
			RowNumber:  rowNumber,
			MobileE164: mobile,
		})
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("%s: CSV contains no data rows", op)
	}

	return rows, nil
}
