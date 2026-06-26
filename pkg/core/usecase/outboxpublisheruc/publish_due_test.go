package outboxpublisheruc

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/stretchr/testify/assert"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type logRecord struct {
	level string
	msg   string
	args  []any
}

type recordingLogger struct {
	mu      *sync.Mutex
	records *[]logRecord
	attrs   []any
}

func newRecordingLogger() *recordingLogger {
	records := make([]logRecord, 0)
	return &recordingLogger{
		mu:      &sync.Mutex{},
		records: &records,
	}
}

func (l *recordingLogger) With(args ...any) corelog.Logger {
	attrs := append(append([]any{}, l.attrs...), args...)
	return &recordingLogger{mu: l.mu, records: l.records, attrs: attrs}
}

func (l *recordingLogger) Debug(msg string, args ...any) { l.append("debug", msg, args...) }
func (l *recordingLogger) Info(msg string, args ...any)  { l.append("info", msg, args...) }
func (l *recordingLogger) Warn(msg string, args ...any)  { l.append("warn", msg, args...) }
func (l *recordingLogger) Error(msg string, args ...any) { l.append("error", msg, args...) }

func (l *recordingLogger) append(level, msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	combined := append(append([]any{}, l.attrs...), args...)
	*l.records = append(*l.records, logRecord{level: level, msg: msg, args: combined})
}

func (l *recordingLogger) snapshot() []logRecord {
	l.mu.Lock()
	defer l.mu.Unlock()

	out := make([]logRecord, len(*l.records))
	copy(out, *l.records)
	return out
}

func TestPublishDue_ContextCancelledBeforeConn_ReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	deps := newOutboxPublisherDeps(t, time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC))
	deps.pool.EXPECT().Conn(ctx, testifymock.Anything).Return(context.Canceled).Once()

	uc := deps.newUseCase("worker-1", 5)

	err := uc.PublishDue(ctx)

	require.ErrorIs(t, err, context.Canceled)
}

func TestPublishDue_TwoMessagesBatch_PartialFailure(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	deps := newOutboxPublisherDeps(t, now)
	msg1 := sampleOutboxMessage()
	msg2 := sampleOutboxMessage()
	msg2.ID = uuid.New()
	publishErr := errors.New("publisher unavailable")
	nextAttemptAt := now.Add(time.Second)

	deps.outbox.EXPECT().Conn(deps.conn).Return(deps.outboxQueryer).Once()
	deps.outboxQueryer.EXPECT().ClaimPending(ctx, "worker-1", 5, now).Return([]model.OutboxMessage{msg1, msg2}, nil).Once()
	deps.publisher.EXPECT().Publish(ctx, msg1.Destination, expectedMessage(msg1)).Return(publishErr).Once()
	deps.outboxQueryer.EXPECT().MarkRetry(ctx, msg1.ID, nextAttemptAt, publishErr.Error()).Return(nil).Once()
	deps.publisher.EXPECT().Publish(ctx, msg2.Destination, expectedMessage(msg2)).Return(nil).Once()
	deps.outboxQueryer.EXPECT().MarkPublished(ctx, msg2.ID, now).Return(nil).Once()
	deps.pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(func(ctx context.Context, handler repo.ConnHandler) error {
		return handler(ctx, deps.conn)
	}).Once()

	uc := deps.newUseCase("worker-1", 5)

	err := uc.PublishDue(ctx)

	require.Error(t, err)
	require.ErrorContains(t, err, publishErr.Error())
}

func TestPublishDue_ReturnsErrMissingWorkerID(t *testing.T) {
	uc := newUseCaseForTest(t, useCaseOptions{workerID: "", batchSize: 1})

	err := uc.PublishDue(context.Background())

	require.ErrorIs(t, err, ErrMissingWorkerID)
}

func TestPublishDue_ReturnsErrInvalidBatchSize(t *testing.T) {
	uc := newUseCaseForTest(t, useCaseOptions{workerID: "worker-1", batchSize: 0})

	err := uc.PublishDue(context.Background())

	require.ErrorIs(t, err, ErrInvalidBatchSize)
}

func TestPublishDue_NoMessages_ReturnsNil(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	deps := newOutboxPublisherDeps(t, now)

	deps.outbox.EXPECT().Conn(deps.conn).Return(deps.outboxQueryer).Once()
	deps.outboxQueryer.EXPECT().ClaimPending(ctx, "worker-1", 5, now).Return([]model.OutboxMessage{}, nil).Once()
	deps.pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(func(ctx context.Context, handler repo.ConnHandler) error {
		return handler(ctx, deps.conn)
	}).Once()

	uc := deps.newUseCase("worker-1", 5)

	err := uc.PublishDue(ctx)

	require.NoError(t, err)
}

func TestPublishDue_SuccessfulPublish_MarksPublished(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	deps := newOutboxPublisherDeps(t, now)
	msg := sampleOutboxMessage()

	deps.outbox.EXPECT().Conn(deps.conn).Return(deps.outboxQueryer).Once()
	deps.outboxQueryer.EXPECT().ClaimPending(ctx, "worker-1", 5, now).Return([]model.OutboxMessage{msg}, nil).Once()
	deps.publisher.EXPECT().Publish(ctx, msg.Destination, expectedMessage(msg)).Return(nil).Once()
	deps.outboxQueryer.EXPECT().MarkPublished(ctx, msg.ID, now).Return(nil).Once()
	deps.pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(func(ctx context.Context, handler repo.ConnHandler) error {
		return handler(ctx, deps.conn)
	}).Once()

	uc := deps.newUseCase("worker-1", 5)

	err := uc.PublishDue(ctx)

	require.NoError(t, err)
}

func TestPublishDue_PublishFails_SchedulesRetry(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	deps := newOutboxPublisherDeps(t, now)
	msg := sampleOutboxMessage()
	msg.AttemptCount = 1
	publishErr := errors.New("publisher unavailable")
	nextAttemptAt := now.Add(time.Second)

	deps.outbox.EXPECT().Conn(deps.conn).Return(deps.outboxQueryer).Once()
	deps.outboxQueryer.EXPECT().ClaimPending(ctx, "worker-1", 5, now).Return([]model.OutboxMessage{msg}, nil).Once()
	deps.publisher.EXPECT().Publish(ctx, msg.Destination, expectedMessage(msg)).Return(publishErr).Once()
	deps.outboxQueryer.EXPECT().MarkRetry(ctx, msg.ID, nextAttemptAt, publishErr.Error()).Return(nil).Once()
	deps.pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(func(ctx context.Context, handler repo.ConnHandler) error {
		return handler(ctx, deps.conn)
	}).Once()

	uc := deps.newUseCase("worker-1", 5)

	err := uc.PublishDue(ctx)

	require.Error(t, err)
	require.ErrorContains(t, err, publishErr.Error())
}

func TestPublishDue_PublishFails_ExhaustedRetries_MarksFailed(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	deps := newOutboxPublisherDeps(t, now)
	msg := sampleOutboxMessage()
	msg.AttemptCount = msg.MaxAttempts
	publishErr := errors.New("publisher unavailable")

	deps.outbox.EXPECT().Conn(deps.conn).Return(deps.outboxQueryer).Once()
	deps.outboxQueryer.EXPECT().ClaimPending(ctx, "worker-1", 5, now).Return([]model.OutboxMessage{msg}, nil).Once()
	deps.publisher.EXPECT().Publish(ctx, msg.Destination, expectedMessage(msg)).Return(publishErr).Once()
	deps.outboxQueryer.EXPECT().MarkFailed(ctx, msg.ID, publishErr.Error()).Return(nil).Once()
	deps.pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(func(ctx context.Context, handler repo.ConnHandler) error {
		return handler(ctx, deps.conn)
	}).Once()

	uc := deps.newUseCase("worker-1", 5)

	err := uc.PublishDue(ctx)

	require.Error(t, err)
	require.ErrorContains(t, err, publishErr.Error())
}

func TestPublishDue_MarkPublishedFails_Logs_ContinuesProcessing(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	deps := newOutboxPublisherDeps(t, now)
	msg1 := sampleOutboxMessage()
	msg2 := sampleOutboxMessage()
	msg2.ID = uuid.New()
	markErr := errors.New("mark published failed")

	deps.outbox.EXPECT().Conn(deps.conn).Return(deps.outboxQueryer).Once()
	deps.outboxQueryer.EXPECT().ClaimPending(ctx, "worker-1", 5, now).Return([]model.OutboxMessage{msg1, msg2}, nil).Once()
	deps.publisher.EXPECT().Publish(ctx, msg1.Destination, expectedMessage(msg1)).Return(nil).Once()
	deps.outboxQueryer.EXPECT().MarkPublished(ctx, msg1.ID, now).Return(markErr).Once()
	deps.publisher.EXPECT().Publish(ctx, msg2.Destination, expectedMessage(msg2)).Return(nil).Once()
	deps.outboxQueryer.EXPECT().MarkPublished(ctx, msg2.ID, now).Return(nil).Once()
	deps.pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(func(ctx context.Context, handler repo.ConnHandler) error {
		return handler(ctx, deps.conn)
	}).Once()

	uc := deps.newUseCase("worker-1", 5)

	err := uc.PublishDue(ctx)

	require.Error(t, err)
	require.ErrorContains(t, err, markErr.Error())

	records := deps.logger.snapshot()
	require.Len(t, records, 2)
	assert.Equal(t, "error", records[0].level)
	assert.Equal(t, "failed to mark outbox message published", records[0].msg)
}

func TestPublishDue_ClaimPendingFails_ReturnsError(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	deps := newOutboxPublisherDeps(t, now)
	claimErr := errors.New("claim failed")

	deps.outbox.EXPECT().Conn(deps.conn).Return(deps.outboxQueryer).Once()
	deps.outboxQueryer.EXPECT().ClaimPending(ctx, "worker-1", 5, now).Return(nil, claimErr).Once()
	deps.pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(func(ctx context.Context, handler repo.ConnHandler) error {
		return handler(ctx, deps.conn)
	}).Once()

	uc := deps.newUseCase("worker-1", 5)

	err := uc.PublishDue(ctx)

	require.Error(t, err)
	require.ErrorContains(t, err, "claim pending outbox messages")
	require.ErrorIs(t, err, claimErr)
}

type useCaseOptions struct {
	workerID  string
	batchSize int
}

func newUseCaseForTest(t *testing.T, opts useCaseOptions) *UseCase {
	t.Helper()
	deps := newOutboxPublisherDeps(t, time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC))
	return deps.newUseCase(opts.workerID, opts.batchSize)
}

type outboxPublisherDeps struct {
	pool          *mock.Pool
	conn          *mock.Conn
	outbox        *mock.Outbox
	outboxQueryer *mock.OutboxConnQueryer
	publisher     *mock.Publisher
	clock         adapterclock.Fixed
	logger        *recordingLogger
}

func newOutboxPublisherDeps(t *testing.T, now time.Time) outboxPublisherDeps {
	t.Helper()
	loc := time.UTC
	return outboxPublisherDeps{
		pool:          mock.NewPool(t),
		conn:          mock.NewConn(t),
		outbox:        mock.NewOutbox(t),
		outboxQueryer: mock.NewOutboxConnQueryer(t),
		publisher:     mock.NewPublisher(t),
		clock:         adapterclock.NewFixed(now, loc),
		logger:        newRecordingLogger(),
	}
}

func (d outboxPublisherDeps) newUseCase(workerID string, batchSize int) *UseCase {
	return New(
		d.pool,
		d.outbox,
		d.publisher,
		d.clock,
		d.logger,
		workerID,
		batchSize,
		time.Second,
		time.Minute,
	)
}

func sampleOutboxMessage() model.OutboxMessage {
	return model.OutboxMessage{
		ID:           uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Destination:  "payments.result",
		EventType:    "payment.completed",
		Payload:      []byte(`{"status":"ok"}`),
		AttemptCount: 1,
		MaxAttempts:  3,
	}
}

func expectedMessage(msg model.OutboxMessage) messaging.Message {
	return messaging.Message{
		Key:   []byte(msg.ID.String()),
		Value: msg.Payload,
		Topic: msg.Destination,
		Headers: map[string][]byte{
			"outbox_id":      []byte(msg.ID.String()),
			"event_type":     []byte(msg.EventType),
			"aggregate_type": nil,
		},
	}
}
