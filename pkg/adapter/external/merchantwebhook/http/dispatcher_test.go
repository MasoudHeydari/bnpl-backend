package http_test

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"

	merchantwebhookhttp "github.com/parstis/bnpl-backend/pkg/adapter/external/merchantwebhook/http"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	webhookpolicy "github.com/parstis/bnpl-backend/pkg/core/policy/webhook"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/webhook"
)

type harness struct {
	dispatcher        *merchantwebhookhttp.Dispatcher
	queryer           *mock.CheckoutSessionConnQueryer
	metrics           *mock.BusinessMetrics
	deliverer         *mock.WebhookDeliverer
	delivererCalls    int
	delivererLast     webhook.Delivery
	delivererResponse error
	merchantID        model.MerchantID
	now               time.Time
}

func newHarness(t *testing.T, plaintext string, decryptErr error, policy webhookpolicy.Policy) *harness {
	t.Helper()

	ctx := testifymock.Anything
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

	pool := mock.NewPool(t)
	conn := mock.NewConn(t)
	checkoutSession := mock.NewCheckoutSession(t)
	queryer := mock.NewCheckoutSessionConnQueryer(t)
	businessMetrics := mock.NewBusinessMetrics(t)
	box := mock.NewSecretBox(t)
	deliverer := mock.NewWebhookDeliverer(t)

	h := &harness{
		queryer:    queryer,
		metrics:    businessMetrics,
		deliverer:  deliverer,
		merchantID: model.MerchantID(uuid.New()),
		now:        now,
	}

	box.EXPECT().
		Encrypt(testifymock.Anything).
		Return("", "", errors.New("unused")).
		Maybe()
	box.EXPECT().
		Decrypt(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ciphertext string, keyID string) (string, error) {
			if decryptErr != nil {
				return "", decryptErr
			}

			if ciphertext != "ct" || keyID != "k1" {
				return "", errors.New("unexpected ciphertext or key id")
			}

			return plaintext, nil
		}).
		Maybe()

	deliverer.EXPECT().
		Deliver(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(_ context.Context, delivery webhook.Delivery) error {
			h.delivererCalls++
			h.delivererLast = delivery

			return h.delivererResponse
		}).
		Maybe()

	pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error { return handler(ctx, conn) },
	).Maybe()
	checkoutSession.EXPECT().Conn(conn).Return(queryer).Maybe()

	dispatcher, err := merchantwebhookhttp.NewDispatcher(
		pool, checkoutSession, box, deliverer, policy,
		adapterclock.NewFixed(now, time.UTC), lognoop.New(), businessMetrics,
	)
	require.NoError(t, err)

	h.dispatcher = dispatcher

	return h
}

func (h *harness) message() messaging.Message {
	return messaging.Message{
		Key:   []byte("session"),
		Value: []byte(`{ "event_id": "sess:bnpl.checkout.completed.v1:completed", "event_type": "bnpl.checkout.completed.v1" }`),
		Headers: map[string][]byte{
			"event_type": []byte("bnpl.checkout.completed.v1"),
			"outbox_id":  []byte(uuid.NewString()),
		},
	}
}

func (h *harness) destination() string { return webhook.MerchantDestination(h.merchantID) }

func destinationRow(rawURL string, enabled bool) model.MerchantWebhookDestination {
	return model.MerchantWebhookDestination{
		WebhookURL:              rawURL,
		Enabled:                 enabled,
		SigningSecretCiphertext: "ct",
		SigningSecretKeyID:      "k1",
	}
}

var prodPolicy = webhookpolicy.Strict()

func TestPublish_DeliversSignedRequestUsingCurrentDestination(t *testing.T) {
	t.Parallel()

	h := newHarness(t, "topsecret", nil, prodPolicy)
	h.queryer.EXPECT().FindWebhookDestination(testifymock.Anything, h.merchantID).
		Return(destinationRow("https://hooks.example.com/bnpl", true), nil).Once()
	h.metrics.EXPECT().WebhookDeliveryCompleted(testifymock.Anything, "delivered").Once()

	require.NoError(t, h.dispatcher.Publish(context.Background(), h.destination(), h.message()))

	require.Equal(t, 1, h.delivererCalls)

	got := h.delivererLast
	wantURL, _ := url.Parse("https://hooks.example.com/bnpl")
	require.Equal(t, wantURL, got.URL)

	const eventID = "sess:bnpl.checkout.completed.v1:completed"

	timestamp := h.now.Unix()
	require.Equal(t, eventID, got.Headers[webhook.HeaderID])
	require.Equal(t, strconv.FormatInt(timestamp, 10), got.Headers[webhook.HeaderTimestamp])
	require.Equal(t, merchantwebhookhttp.Sign([]byte("topsecret"), eventID, timestamp, got.Body), got.Headers[webhook.HeaderSignature])
	require.JSONEq(t, string(h.message().Value), string(got.Body))
	require.NotContains(t, string(got.Body), " ", "body must be compacted before signing")
}

func TestPublish_DisabledDestinationIsPermanentAndDoesNotDeliver(t *testing.T) {
	t.Parallel()

	h := newHarness(t, "s", nil, prodPolicy)
	h.queryer.EXPECT().FindWebhookDestination(testifymock.Anything, h.merchantID).
		Return(destinationRow("https://hooks.example.com/bnpl", false), nil).Once()
	h.metrics.EXPECT().WebhookDeliveryCompleted(testifymock.Anything, "destination_disabled").Once()

	err := h.dispatcher.Publish(context.Background(), h.destination(), h.message())

	require.ErrorIs(t, err, merchantwebhookhttp.ErrDestinationDisabled)
	require.True(t, messaging.IsNonRetryable(err))
	require.Zero(t, h.delivererCalls)
}

func TestPublish_MissingDestinationIsPermanent(t *testing.T) {
	t.Parallel()

	h := newHarness(t, "s", nil, prodPolicy)
	h.queryer.EXPECT().FindWebhookDestination(testifymock.Anything, h.merchantID).
		Return(model.MerchantWebhookDestination{}, cerr.ResourceNotFound(errors.New("none"))).Once()
	h.metrics.EXPECT().WebhookDeliveryCompleted(testifymock.Anything, "destination_missing").Once()

	err := h.dispatcher.Publish(context.Background(), h.destination(), h.message())

	require.ErrorIs(t, err, merchantwebhookhttp.ErrDestinationMissing)
	require.True(t, messaging.IsNonRetryable(err))
}

func TestPublish_DatabaseErrorIsRetryable(t *testing.T) {
	t.Parallel()

	h := newHarness(t, "s", nil, prodPolicy)
	dbErr := errors.New("connection reset")
	h.queryer.EXPECT().FindWebhookDestination(testifymock.Anything, h.merchantID).
		Return(model.MerchantWebhookDestination{}, dbErr).Once()
	h.metrics.EXPECT().WebhookDeliveryCompleted(testifymock.Anything, "retryable_failure").Once()

	err := h.dispatcher.Publish(context.Background(), h.destination(), h.message())

	require.ErrorIs(t, err, dbErr)
	require.False(t, messaging.IsNonRetryable(err))
	require.Zero(t, h.delivererCalls)
}

func TestPublish_URLPolicyViolationIsPermanent(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"http://hooks.example.com/bnpl",
		"https://127.0.0.1/bnpl",
		"https://169.254.169.254/latest",
		"https://user@hooks.example.com/",
		"/relative",
	} {
		h := newHarness(t, "s", nil, prodPolicy)
		h.queryer.EXPECT().FindWebhookDestination(testifymock.Anything, h.merchantID).
			Return(destinationRow(raw, true), nil).Once()
		h.metrics.EXPECT().WebhookDeliveryCompleted(testifymock.Anything, "invalid_destination").Once()

		err := h.dispatcher.Publish(context.Background(), h.destination(), h.message())

		require.Error(t, err, raw)
		require.True(t, messaging.IsNonRetryable(err), raw)
		require.Zero(t, h.delivererCalls, raw)
	}
}

func TestPublish_MissingSigningSecretIsPermanent(t *testing.T) {
	t.Parallel()

	h := newHarness(t, "s", nil, prodPolicy)
	row := destinationRow("https://hooks.example.com/bnpl", true)
	row.SigningSecretCiphertext, row.SigningSecretKeyID = "", ""
	h.queryer.EXPECT().FindWebhookDestination(testifymock.Anything, h.merchantID).Return(row, nil).Once()
	h.metrics.EXPECT().WebhookDeliveryCompleted(testifymock.Anything, "signing_unavailable").Once()

	err := h.dispatcher.Publish(context.Background(), h.destination(), h.message())

	require.ErrorIs(t, err, merchantwebhookhttp.ErrSigningSecretMissing)
	require.True(t, messaging.IsNonRetryable(err))
}

func TestPublish_DecryptFailureIsRetryable(t *testing.T) {
	t.Parallel()

	decryptErr := errors.New("unknown key")
	h := newHarness(t, "", decryptErr, prodPolicy)
	h.queryer.EXPECT().FindWebhookDestination(testifymock.Anything, h.merchantID).
		Return(destinationRow("https://hooks.example.com/bnpl", true), nil).Once()
	h.metrics.EXPECT().WebhookDeliveryCompleted(testifymock.Anything, "signing_unavailable").Once()

	err := h.dispatcher.Publish(context.Background(), h.destination(), h.message())

	require.ErrorIs(t, err, decryptErr)
	require.False(t, messaging.IsNonRetryable(err))
	require.Zero(t, h.delivererCalls)
}

func TestPublish_DelivererOutcomesMapToRetryability(t *testing.T) {
	t.Parallel()

	t.Run("retryable", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t, "s", nil, prodPolicy)
		h.delivererResponse = errors.New("status 503")
		h.queryer.EXPECT().FindWebhookDestination(testifymock.Anything, h.merchantID).
			Return(destinationRow("https://hooks.example.com/bnpl", true), nil).Once()
		h.metrics.EXPECT().WebhookDeliveryCompleted(testifymock.Anything, "retryable_failure").Once()

		err := h.dispatcher.Publish(context.Background(), h.destination(), h.message())

		require.Error(t, err)
		require.False(t, messaging.IsNonRetryable(err))
	})

	t.Run("permanent", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t, "s", nil, prodPolicy)
		h.delivererResponse = messaging.NonRetryable(errors.New("status 400"))
		h.queryer.EXPECT().FindWebhookDestination(testifymock.Anything, h.merchantID).
			Return(destinationRow("https://hooks.example.com/bnpl", true), nil).Once()
		h.metrics.EXPECT().WebhookDeliveryCompleted(testifymock.Anything, "permanent_failure").Once()

		err := h.dispatcher.Publish(context.Background(), h.destination(), h.message())

		require.Error(t, err)
		require.True(t, messaging.IsNonRetryable(err))
	})
}

func TestPublish_MalformedMessagesArePermanent(t *testing.T) {
	t.Parallel()

	h := newHarness(t, "s", nil, prodPolicy)
	h.metrics.EXPECT().WebhookDeliveryCompleted(testifymock.Anything, "invalid_message").Times(3)

	bad := h.message()
	bad.Value = []byte(`{"event_type":"x"}`)
	require.True(t, messaging.IsNonRetryable(h.dispatcher.Publish(context.Background(), h.destination(), bad)))

	bad.Value = []byte(`not json`)
	require.True(t, messaging.IsNonRetryable(h.dispatcher.Publish(context.Background(), h.destination(), bad)))

	require.True(t, messaging.IsNonRetryable(h.dispatcher.Publish(context.Background(), "webhook://merchant/not-a-uuid", h.message())))
	require.Zero(t, h.delivererCalls)
}

func TestNew_RequiresDependencies(t *testing.T) {
	t.Parallel()

	_, err := merchantwebhookhttp.NewDispatcher(nil, nil, nil, nil, webhookpolicy.Policy{}, nil, nil, nil)
	require.Error(t, err)
}
