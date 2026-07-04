//go:build e2e || legacy_e2e

package e2e_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

const (
	defaultPostgresImage   = "postgres:18-alpine"
	defaultStartupTimeout  = 5 * time.Minute
	defaultShutdownTimeout = 30 * time.Second
	defaultCommandTimeout  = 5 * time.Minute
)

var (
	e2eBaseURL string
	e2eClient  = &http.Client{Timeout: 5 * time.Second}
	testDB     *pgxpool.Pool
)

// applicationProcess owns the E2E application subprocess and its captured logs.
type applicationProcess struct {
	cmd        *exec.Cmd
	tempDir    string
	binaryPath string
	stdout     bytes.Buffer
	stderr     bytes.Buffer
	waitResult chan error
}

func TestMain(m *testing.M) {
	os.Exit(runTestMain(m))
}

// runTestMain prepares the test environment, starts the real application binary,
// runs the suite, and then shuts down all resources.
//
// Lifecycle order (each step's cleanup is registered via defer immediately
// after the step that acquires the resource succeeds, so every failure path
// below it still unwinds cleanly):
//
//  1. load env
//  2. start PostgreSQL container
//  3. build binary
//  4. initialize schema
//  5. run migrations
//  6. open administrative test pool
//  7. validate reset table policy once (schema-drift guard for resetDatabase)
//  8. start application / wait for readiness
//  9. run tests
//  10. stop application
//  11. close test pool
//  12. terminate container
func runTestMain(m *testing.M) (code int) {
	root, err := repositoryRoot()
	if err != nil {
		log.Printf("resolve E2E repository root: %v", err)
		return 1
	}

	if err = loadEnv(filepath.Join(root, "test", "e2e", ".env")); err != nil {
		log.Printf("load E2E environment: %v", err)
		return 1
	}

	e2eBaseURL = strings.TrimRight(os.Getenv("E2E_BASE_URL"), "/")
	if e2eBaseURL == "" {
		log.Print("E2E_BASE_URL is required")
		return 1
	}

	cleanupDB, err := setupDB()
	if err != nil {
		log.Printf("start E2E PostgreSQL container: %v", err)
		return 1
	}

	defer func() {
		if cleanupErr := cleanupDB(); cleanupErr != nil {
			log.Printf("terminate E2E PostgreSQL container: %v", cleanupErr)
			code = 1
		}
	}()

	app, err := newApplicationProcess()
	if err != nil {
		log.Printf("prepare E2E application: %v", err)
		return 1
	}

	defer app.cleanup()

	if err = app.build(root); err != nil {
		log.Printf("build E2E application: %v", err)
		app.printLogs()
		return 1
	}

	if err = app.initSchema(root); err != nil {
		log.Printf("initialize E2E database schema: %v", err)
		app.printLogs()
		return 1
	}

	if err = app.migrate(root); err != nil {
		log.Printf("migrate E2E database: %v", err)
		app.printLogs()
		return 1
	}

	dbCtx, dbCancel := context.WithTimeout(context.Background(), 30*time.Second)
	testDB, err = openTestDB(dbCtx)
	dbCancel()
	if err != nil {
		log.Printf("open E2E administrative database pool: %v", err)
		return 1
	}

	defer testDB.Close()

	policyCtx, policyCancel := context.WithTimeout(context.Background(), 30*time.Second)
	err = validateResetTablePolicyAgainstSchema(policyCtx)
	policyCancel()
	if err != nil {
		log.Printf("validate E2E reset table policy: %v", err)
		return 1
	}

	if err = app.start(root); err != nil {
		log.Printf("start E2E application: %v", err)
		app.printLogs()
		return 1
	}

	code = m.Run()

	if err = app.stop(); err != nil {
		fmt.Fprintf(os.Stderr, "stop E2E application: %v\n", err)
		code = 1
	}

	if code != 0 {
		app.printLogs()
	}

	return code
}

// repositoryRoot resolves the repository root independently of the process's
// current working directory, so the suite behaves identically whether it is
// launched from the repository root, from test/e2e, from a GoLand test
// configuration, or from CI.
func repositoryRoot() (string, error) {
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		return "", fmt.Errorf("resolve go module file via 'go env GOMOD': %w", err)
	}

	gomod := strings.TrimSpace(string(out))
	if gomod == "" || gomod == os.DevNull {
		return "", errors.New("'go env GOMOD' returned no module file; are the E2E tests running inside the bnpl-backend module?")
	}

	return filepath.Dir(gomod), nil
}

// newApplicationProcess allocates the temporary directory the E2E binary is
// built into. The caller owns cleanup via applicationProcess.cleanup.
func newApplicationProcess() (*applicationProcess, error) {
	tempDir, err := os.MkdirTemp("", "bnpl-e2e-*")
	if err != nil {
		return nil, fmt.Errorf("create E2E temp directory: %w", err)
	}

	return &applicationProcess{
		tempDir:    tempDir,
		binaryPath: filepath.Join(tempDir, "bnpl"),
		waitResult: make(chan error, 1),
	}, nil
}

// build compiles the production binary from root into the process's temp directory.
func (a *applicationProcess) build(root string) error {
	if err := a.runCommand(
		root,
		"go",
		"build",
		"-tags=go_json,nomsgpack,development",
		"-o",
		a.binaryPath,
		"./cmd/bnpl",
	); err != nil {
		return fmt.Errorf("build E2E binary: %w", err)
	}

	return nil
}

// initSchema recreates the managed schema via the same binary and command
// path used by the real application, instead of applying schema SQL directly.
func (a *applicationProcess) initSchema(root string) error {
	if err := a.runCommand(root, a.binaryPath, "db", "init-dev"); err != nil {
		return fmt.Errorf("initialize E2E database: %w", err)
	}

	return nil
}

// migrate applies pending migrations via the same binary and command path
// used by the real application.
func (a *applicationProcess) migrate(root string) error {
	if err := a.runCommand(root, a.binaryPath, "db", "migrate"); err != nil {
		return fmt.Errorf("migrate E2E database: %w", err)
	}

	return nil
}

// start spawns the long-running application process and blocks until it
// reports readiness. On failure, the spawned process (if any) is killed and
// reaped before returning so no subprocess is leaked.
func (a *applicationProcess) start(root string) error {
	a.cmd = exec.Command(a.binaryPath)
	a.cmd.Dir = root
	a.cmd.Env = os.Environ()
	a.cmd.Stdout = &a.stdout
	a.cmd.Stderr = &a.stderr

	if err := a.cmd.Start(); err != nil {
		return fmt.Errorf("start E2E application: %w", err)
	}

	go func() {
		a.waitResult <- a.cmd.Wait()
	}()

	if err := a.waitUntilReady(); err != nil {
		_ = a.killAndWait()
		return err
	}

	return nil
}

// runCommand executes a setup command with a bounded timeout and captured logs.
func (a *applicationProcess) runCommand(root, name string, args ...string) error {
	timeout, err := envDurationOrDefault("E2E_COMMAND_TIMEOUT", defaultCommandTimeout)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = root
	cmd.Env = os.Environ()
	cmd.Stdout = &a.stdout
	cmd.Stderr = &a.stderr

	if err = cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%s timed out after %s", name, timeout)
		}

		return fmt.Errorf(
			"%s %s: %w\n%s",
			name,
			strings.Join(args, " "),
			err,
			a.stderr.String(),
		)
	}

	return nil
}

// waitUntilReady polls the production readiness endpoint until startup succeeds,
// the process exits, or the configured timeout expires.
func (a *applicationProcess) waitUntilReady() error {
	timeout, err := envDurationOrDefault("E2E_STARTUP_TIMEOUT", 30*time.Second)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case waitErr := <-a.waitResult:
			if waitErr == nil {
				return errors.New("E2E application exited during startup")
			}

			return fmt.Errorf("E2E application exited during startup: %w", waitErr)

		case <-ctx.Done():
			return fmt.Errorf(
				"E2E application was not ready within %s: %w",
				timeout,
				ctx.Err(),
			)

		case <-ticker.C:
			request, requestErr := http.NewRequestWithContext(
				ctx,
				http.MethodGet,
				e2eBaseURL+"/healthz/ready",
				nil,
			)
			if requestErr != nil {
				return fmt.Errorf("create readiness request: %w", requestErr)
			}

			response, requestErr := e2eClient.Do(request)
			if requestErr != nil {
				continue
			}

			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
	}
}

// stop requests graceful shutdown and verifies that the process exits in time.
func (a *applicationProcess) stop() error {
	if a.cmd == nil || a.cmd.Process == nil {
		return nil
	}

	if err := a.cmd.Process.Signal(syscall.SIGTERM); err != nil &&
		!errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("send SIGTERM: %w", err)
	}

	timeout, err := envDurationOrDefault("E2E_SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return err
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case waitErr := <-a.waitResult:
		if waitErr != nil {
			return fmt.Errorf("application process exit: %w", waitErr)
		}

		return nil

	case <-timer.C:
		killErr := a.killAndWait()
		if killErr != nil {
			return errors.Join(
				fmt.Errorf("application did not stop gracefully within %s", timeout),
				killErr,
			)
		}

		return fmt.Errorf("application did not stop gracefully within %s", timeout)
	}
}

// killAndWait forcefully stops the subprocess and reaps it.
func (a *applicationProcess) killAndWait() error {
	if a.cmd == nil || a.cmd.Process == nil {
		return nil
	}

	if err := a.cmd.Process.Kill(); err != nil &&
		!errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("kill application process: %w", err)
	}

	select {
	case <-a.waitResult:
		return nil

	case <-time.After(defaultShutdownTimeout):
		return errors.New("timed out waiting for killed application process")
	}
}

// cleanup removes the temporary binary directory. Safe to call unconditionally,
// including when build never reached the point of writing a binary.
func (a *applicationProcess) cleanup() {
	_ = os.RemoveAll(a.tempDir)
}

func (a *applicationProcess) printLogs() {
	if a.stdout.Len() > 0 {
		fmt.Fprintf(
			os.Stderr,
			"\nE2E application stdout:\n%s\n",
			a.stdout.String(),
		)
	}

	if a.stderr.Len() > 0 {
		fmt.Fprintf(
			os.Stderr,
			"\nE2E application stderr:\n%s\n",
			a.stderr.String(),
		)
	}
}

// loadEnv loads simple KEY=VALUE entries from path.
//
// Keys defined in the E2E env file override ambient values exported by Make's
// root `.env` include so the suite listens on the intended GIN_PORT / base URL.
// Database connection settings are still replaced later by setupDB.
func loadEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open E2E env %q: %w", path, err)
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, value, found := strings.Cut(line, "=")
		if !found || strings.TrimSpace(key) == "" {
			return fmt.Errorf("invalid E2E env line %d", lineNo)
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		if len(value) >= 2 &&
			((value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}

		if err = os.Setenv(key, value); err != nil {
			return fmt.Errorf("set E2E env %q: %w", key, err)
		}
	}

	if err = scanner.Err(); err != nil {
		return fmt.Errorf("read E2E env: %w", err)
	}

	return nil
}

// setupDB starts an isolated PostgreSQL container. Credentials and the image are
// read from the E2E environment; the runtime host and mapped port are exported
// for the application subprocess.
func setupDB() (func() error, error) {
	dbName, err := requiredEnv("POSTGRES_DB")
	if err != nil {
		return nil, err
	}

	dbUser, err := requiredEnv("POSTGRES_USER")
	if err != nil {
		return nil, err
	}

	dbPassword, err := requiredEnv("POSTGRES_PASSWORD")
	if err != nil {
		return nil, err
	}

	postgresImage := envOrDefault("POSTGRES_IMAGE", defaultPostgresImage)

	startupCtx, cancelStartup := context.WithTimeout(
		context.Background(),
		defaultStartupTimeout,
	)
	defer cancelStartup()

	postgresContainer, err := postgres.Run(
		startupCtx,
		postgresImage,
		postgres.WithDatabase(dbName),
		postgres.WithUsername(dbUser),
		postgres.WithPassword(dbPassword),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		return nil, fmt.Errorf("start PostgreSQL container: %w", err)
	}

	cleanup := func() error {
		shutdownCtx, cancelShutdown := context.WithTimeout(
			context.Background(),
			defaultShutdownTimeout,
		)
		defer cancelShutdown()

		if terminateErr := testcontainers.TerminateContainer(
			postgresContainer,
			testcontainers.StopContext(shutdownCtx),
		); terminateErr != nil {
			return fmt.Errorf(
				"terminate PostgreSQL container: %w",
				terminateErr,
			)
		}

		return nil
	}

	host, err := postgresContainer.Host(startupCtx)
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("get PostgreSQL container host: %w", err),
			cleanup(),
		)
	}

	mappedPort, err := postgresContainer.MappedPort(startupCtx, "5432/tcp")
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("get PostgreSQL mapped port: %w", err),
			cleanup(),
		)
	}

	connectionString, err := postgresContainer.ConnectionString(
		startupCtx,
		"sslmode=disable",
	)
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("get PostgreSQL connection string: %w", err),
			cleanup(),
		)
	}

	if err = configurePostgresEnv(
		host,
		mappedPort.Port(),
		dbName,
		dbUser,
		dbPassword,
		connectionString,
	); err != nil {
		return nil, errors.Join(err, cleanup())
	}

	return cleanup, nil
}

func openTestDB(ctx context.Context) (*pgxpool.Pool, error) {
	dsn, err := requiredEnv("E2E_POSTGRES_DSN")
	if err != nil {
		return nil, err
	}

	pgConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse E2E postgres config: %w", err)
	}

	pgConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, execErr := conn.Exec(ctx, `SET search_path TO bnpl, public`)
		return execErr
	}

	pool, err := pgxpool.NewWithConfig(ctx, pgConfig)
	if err != nil {
		return nil, fmt.Errorf("create E2E database pool: %w", err)
	}

	if err = pool.Ping(ctx); err != nil {
		pool.Close()

		return nil, fmt.Errorf("ping E2E database: %w", err)
	}

	return pool, nil
}

// mutableE2ETables lists every bnpl-schema table that may hold
// request-generated business data touched by this E2E package. resetDatabase
// truncates exactly these tables before each test.
var mutableE2ETables = []string{
	"decisions",
	"customer_profiles",
	"merchant_profiles",
	"merchant_branch_profiles",
	"merchant_pos_terminals",
	"merchant_online_stores",
	"merchant_return_destinations",
	"credit_accounts",
	"credit_transactions",
	"contracts",
	"installments",
	"installment_charges",
	"payment_attempts",
	"payment_attempt_items",
	"checkout_decision_allocations",
	"checkout_sessions",
	"otp_challenges",
	"otp_rate_limits",
	"integration_outbox",
	"integration_inbox",
	"integration_inbox_conflicts",
	"saga_instances",
	"saga_forward_step_completions",
}

// preservedE2ETables lists every bnpl-schema table that must survive a
// reset: schema metadata and any static/reference data. "migrations" records
// which migrations have been applied and is not business state. The schema
// currently defines no separate static lookup/reference tables (enum-like
// values are enforced via CHECK constraints), so there is nothing else here.
var preservedE2ETables = []string{
	"migrations",
	"policy_assignments",
	"policy_assignment_imports",
	"policy_assignment_import_items",
	"merchant_webhook_destinations",
	"merchant_terminals",
	"merchant_terminal_credentials",
	"merchant_terminal_issuance_deliveries",
	"merchant_terminal_permissions",
}

// resetDatabase truncates every known mutable business table before each
// test, preserving bnpl.migrations so migration metadata never needs to be
// reapplied mid-suite. It fails the test clearly if the schema was never
// initialized instead of silently truncating nothing.
//
// This only truncates the configured mutableE2ETables list; it does not
// re-validate that list against the live schema on every call. That drift
// check runs once, in runTestMain, via validateResetTablePolicyAgainstSchema.
func resetDatabase(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer cancel()

	if testDB == nil {
		t.Fatal("E2E database pool is not initialized")
	}

	requireMigrationsTable(ctx, t)

	identifiers := make([]string, 0, len(mutableE2ETables))
	for _, tableName := range mutableE2ETables {
		identifiers = append(identifiers, pgx.Identifier{"bnpl", tableName}.Sanitize())
	}

	truncateSQL := fmt.Sprintf(
		"TRUNCATE TABLE %s RESTART IDENTITY CASCADE",
		strings.Join(identifiers, ", "),
	)

	if _, err := testDB.Exec(ctx, truncateSQL); err != nil {
		t.Fatalf("truncate E2E database tables: %v", err)
	}
}

// requireMigrationsTable fails the test clearly when bnpl.migrations is
// missing, which means the E2E database was never initialized by the
// application binary (db init-dev + db migrate never ran, or ran against a
// different database than testDB is connected to).
func requireMigrationsTable(ctx context.Context, t *testing.T) {
	t.Helper()

	var exists bool
	err := testDB.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_catalog.pg_tables
			WHERE schemaname = 'bnpl' AND tablename = 'migrations'
		)
	`).Scan(&exists)
	if err != nil {
		t.Fatalf("check E2E migrations table: %v", err)
	}

	if !exists {
		t.Fatal("E2E database is not initialized: bnpl.migrations table is missing")
	}
}

// resetTablePolicyViolations describes every detected drift between the
// configured reset policy (mutableE2ETables / preservedE2ETables) and the
// actual base tables present in the bnpl schema.
type resetTablePolicyViolations struct {
	// Unclassified holds actual schema tables absent from both policy lists.
	Unclassified []string
	// MissingFromSchema holds configured tables that do not actually exist.
	MissingFromSchema []string
	// DuplicateMutable holds names repeated within mutableE2ETables.
	DuplicateMutable []string
	// DuplicatePreserved holds names repeated within preservedE2ETables.
	DuplicatePreserved []string
	// OverlappingBoth holds names present in both mutableE2ETables and
	// preservedE2ETables.
	OverlappingBoth []string
}

func (v resetTablePolicyViolations) isEmpty() bool {
	return len(v.Unclassified) == 0 &&
		len(v.MissingFromSchema) == 0 &&
		len(v.DuplicateMutable) == 0 &&
		len(v.DuplicatePreserved) == 0 &&
		len(v.OverlappingBoth) == 0
}

// Error renders every violation as a single clear diagnostic.
func (v resetTablePolicyViolations) Error() string {
	var b strings.Builder

	writeIfNotEmpty := func(label string, names []string) {
		if len(names) == 0 {
			return
		}

		fmt.Fprintf(&b, "%s: %s\n", label, strings.Join(names, ", "))
	}

	writeIfNotEmpty("unclassified tables (add to mutableE2ETables or preservedE2ETables)", v.Unclassified)
	writeIfNotEmpty("configured tables missing from the bnpl schema", v.MissingFromSchema)
	writeIfNotEmpty("duplicate entries in mutableE2ETables", v.DuplicateMutable)
	writeIfNotEmpty("duplicate entries in preservedE2ETables", v.DuplicatePreserved)
	writeIfNotEmpty("tables present in both mutableE2ETables and preservedE2ETables", v.OverlappingBoth)

	return strings.TrimRight(b.String(), "\n")
}

// checkResetTablePolicy validates actualTables (every base table currently in
// the bnpl schema) against the configured mutable/preserved reset policy
// lists. It has no database dependency, so it can be exercised directly with
// synthetic inputs to prove the drift/duplicate/overlap detection works,
// without needing a live schema change to do so.
//
// It enforces:
//  1. every actual table appears in exactly one of mutable/preserved;
//  2. every configured mutable or preserved table actually exists;
//  3. no table appears in both lists;
//  4. duplicate entries within either list are rejected.
func checkResetTablePolicy(actualTables, mutable, preserved []string) error {
	var violations resetTablePolicyViolations

	mutableCount := make(map[string]int, len(mutable))
	for _, name := range mutable {
		mutableCount[name]++
	}

	preservedCount := make(map[string]int, len(preserved))
	for _, name := range preserved {
		preservedCount[name]++
	}

	for name, n := range mutableCount {
		if n > 1 {
			violations.DuplicateMutable = append(violations.DuplicateMutable, name)
		}
	}

	for name, n := range preservedCount {
		if n > 1 {
			violations.DuplicatePreserved = append(violations.DuplicatePreserved, name)
		}
	}

	configured := make(map[string]struct{}, len(mutableCount)+len(preservedCount))
	for name := range mutableCount {
		configured[name] = struct{}{}
		if _, ok := preservedCount[name]; ok {
			violations.OverlappingBoth = append(violations.OverlappingBoth, name)
		}
	}
	for name := range preservedCount {
		configured[name] = struct{}{}
	}

	actual := make(map[string]struct{}, len(actualTables))
	for _, name := range actualTables {
		actual[name] = struct{}{}
	}

	for name := range actual {
		if _, ok := configured[name]; !ok {
			violations.Unclassified = append(violations.Unclassified, name)
		}
	}

	for name := range configured {
		if _, ok := actual[name]; !ok {
			violations.MissingFromSchema = append(violations.MissingFromSchema, name)
		}
	}

	sort.Strings(violations.Unclassified)
	sort.Strings(violations.MissingFromSchema)
	sort.Strings(violations.DuplicateMutable)
	sort.Strings(violations.DuplicatePreserved)
	sort.Strings(violations.OverlappingBoth)

	if violations.isEmpty() {
		return nil
	}

	return violations
}

// validateResetTablePolicyAgainstSchema queries every base table currently in
// the bnpl schema and validates it against mutableE2ETables/
// preservedE2ETables via checkResetTablePolicy.
//
// Intended to run exactly once, in runTestMain, after migrations have been
// applied and the administrative test pool has been opened but before the
// application starts and tests run. This turns schema drift (a migration
// adding a table without updating the reset policy) into a loud startup
// failure instead of a silent data leak between tests. resetDatabase itself
// does not repeat this check before every test.
func validateResetTablePolicyAgainstSchema(ctx context.Context) error {
	rows, err := testDB.Query(ctx, `
		SELECT tablename
		FROM pg_catalog.pg_tables
		WHERE schemaname = 'bnpl'
		ORDER BY tablename
	`)
	if err != nil {
		return fmt.Errorf("list bnpl schema tables: %w", err)
	}

	defer rows.Close()

	actualTables := make([]string, 0)
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return fmt.Errorf("scan bnpl schema table name: %w", err)
		}

		actualTables = append(actualTables, name)
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("iterate bnpl schema tables: %w", err)
	}

	if err = checkResetTablePolicy(actualTables, mutableE2ETables, preservedE2ETables); err != nil {
		return fmt.Errorf("E2E reset table policy has drifted from the bnpl schema: %w", err)
	}

	return nil
}

// configurePostgresEnv overrides runtime connection values with the host and
// mapped port allocated by Testcontainers.
//
// Admin and normal pgpass entries must use distinct usernames: the container
// bootstrap user (admin) and the application role (bnpl). init-dev looks up the
// normal-role password via GetRolePassword(NormalRole) when creating that role.
func configurePostgresEnv(
	host string,
	port string,
	dbName string,
	dbUser string,
	dbPassword string,
	connectionString string,
) error {
	adminPgpass := fmt.Sprintf(
		"%s:%s:%s:%s:%s",
		host,
		port,
		dbName,
		dbUser,
		dbPassword,
	)
	normalPgpass := fmt.Sprintf(
		"%s:%s:%s:%s:%s",
		host,
		port,
		dbName,
		string(repo.NormalRole),
		dbPassword,
	)

	values := map[string]string{
		"E2E_POSTGRES_DSN":       connectionString,
		"POSTGRES_DSN":           connectionString,
		"POSTGRES_HOST":          host,
		"POSTGRES_PORT":          port,
		"POSTGRES_DB":            dbName,
		"POSTGRES_USER":          dbUser,
		"POSTGRES_PASSWORD":      dbPassword,
		"POSTGRES_ADMIN_PGPASS":  adminPgpass,
		"POSTGRES_NORMAL_PGPASS": normalPgpass,
	}

	for name, value := range values {
		if err := os.Setenv(name, value); err != nil {
			return fmt.Errorf("set %s: %w", name, err)
		}
	}

	return nil
}

func envDurationOrDefault(name string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s=%q: %w", name, value, err)
	}

	if duration <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", name)
	}

	return duration, nil
}

func requiredEnv(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf(
			"required environment variable %q is not set",
			name,
		)
	}

	return value, nil
}

func envOrDefault(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}

	return value
}
