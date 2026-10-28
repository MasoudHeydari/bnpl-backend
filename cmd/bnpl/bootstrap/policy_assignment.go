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
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/policyassignmentuc"
)

// ImportTrustedPolicyAssignments runs the operational one-shot trusted cohort
// importer from an authoritative CSV. Membership trust is owned by the CSV
// operator process: BNPL does not inquire Identity Service during import. The
// command does not start HTTP listeners, messaging consumers, or background
// workers and therefore remains independent of the long-running process lifecycle.
func ImportTrustedPolicyAssignments(sourceFile, actor string) (policyassignmentuc.ImportTrustedOutput, error) {
	const op = "bootstrap.ImportTrustedPolicyAssignments"

	subjects, err := readTrustedSubjectsCSV(sourceFile)
	if err != nil {
		return policyassignmentuc.ImportTrustedOutput{}, fmt.Errorf("%s: %w", op, err)
	}

	ctx := context.Background()

	c, err := config.Load(cfgPath)
	if err != nil {
		return policyassignmentuc.ImportTrustedOutput{}, fmt.Errorf("%s: load config: %w", op, err)
	}

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

	uc, err := policyassignmentuc.New(
		pool,
		policyassignmentrp.New(),
		policyassignmentimportrp.New(),
		clk,
		logger,
	)
	if err != nil {
		return policyassignmentuc.ImportTrustedOutput{}, fmt.Errorf("%s: initialize policy assignment use case: %w", op, err)
	}

	return uc.ImportTrusted(ctx, policyassignmentuc.ImportTrustedInput{
		SourceFile: sourceFile,
		CreatedBy:  actor,
		Subjects:   subjects,
	})
}

func readTrustedSubjectsCSV(path string) ([]policyassignmentuc.ImportSubject, error) {
	const op = "bootstrap.readTrustedSubjectsCSV"

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
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("%s: read header: %w", op, err)
	}

	if len(header) != 1 || strings.TrimSpace(header[0]) != "subject_id" {
		return nil, fmt.Errorf("%s: header must be exactly: subject_id", op)
	}

	rows := make([]policyassignmentuc.ImportSubject, 0)
	for rowNumber := 2; ; rowNumber++ {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("%s: read row %d: %w", op, rowNumber, err)
		}

		if len(record) != 1 {
			return nil, fmt.Errorf("%s: row %d must contain exactly one subject_id", op, rowNumber)
		}

		subjectID := strings.TrimSpace(record[0])
		if subjectID == "" {
			return nil, fmt.Errorf("%s: row %d has empty subject_id", op, rowNumber)
		}

		rows = append(rows, policyassignmentuc.ImportSubject{
			RowNumber: rowNumber,
			SubjectID: subjectID,
		})
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("%s: CSV contains no data rows", op)
	}

	return rows, nil
}
