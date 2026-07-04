package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
	"github.com/parstis/bnpl-backend/pkg/core/metrics"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	webhookpolicy "github.com/parstis/bnpl-backend/pkg/core/policy/webhook"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/secretbox"
	"github.com/parstis/bnpl-backend/pkg/core/webhook"
)

// Delivery outcome labels recorded as business metrics and structured logs.
const (
	outcomeDelivered           = "delivered"
	outcomeRetryableFailure    = "retryable_failure"
	outcomePermanentFailure    = "permanent_failure"
	outcomeDestinationDisabled = "destination_disabled"
	outcomeDestinationMissing  = "destination_missing"
	outcomeInvalidDestination  = "invalid_destination"
	outcomeSigningUnavailable  = "signing_unavailable"
	outcomeInvalidMessage      = "invalid_message"
)

// Permanent, non-retryable dispatch failures. Each is wrapped with
// messaging.NonRetryable so the outbox publisher marks the row failed instead of
// scheduling another attempt.
var (
	// ErrDestinationDisabled means the merchant disabled the webhook destination
	// after the event was enqueued. The event is intentionally not delivered.
	ErrDestinationDisabled = errors.New("webhook destination is disabled")

	// ErrDestinationMissing means the merchant has no webhook destination.
	ErrDestinationMissing = errors.New("webhook destination is not registered")

	// ErrSigningSecretMissing means the destination has no signing secret, so a
	// signed request cannot be produced.
	ErrSigningSecretMissing = errors.New("webhook signing secret is not configured")

	// ErrInvalidMessage means the outbox message cannot be turned into a webhook.
	ErrInvalidMessage = errors.New("invalid webhook outbox message")
)

// Dispatcher delivers webhook outbox messages over HTTP. It implements
// messaging.Publisher so it can be composed with the Kafka publisher behind the
// existing outbox publisher.
type Dispatcher struct {
	pool            repo.Pool
	checkoutSession repo.CheckoutSession
	secrets         secretbox.Box
	deliverer       webhook.Deliverer
	policy          webhookpolicy.Policy
	clock           clock.Clock
	logger          log.Logger
	businessMetrics metrics.BusinessMetrics
}

var _ messaging.Publisher = (*Dispatcher)(nil)

// NewDispatcher constructs an HTTP webhook dispatcher. Every dependency is required.
func NewDispatcher(
	pool repo.Pool,
	checkoutSession repo.CheckoutSession,
	secrets secretbox.Box,
	deliverer webhook.Deliverer,
	policy webhookpolicy.Policy,
	clk clock.Clock,
	logger log.Logger,
	businessMetrics metrics.BusinessMetrics,
) (*Dispatcher, error) {
	const op = "merchantwebhookhttp.NewDispatcher"

	if pool == nil {
		return nil, fmt.Errorf("%s: pool is required", op)
	}

	if checkoutSession == nil {
		return nil, fmt.Errorf("%s: checkout session repository is required", op)
	}

	if secrets == nil {
		return nil, fmt.Errorf("%s: secret box is required", op)
	}

	if deliverer == nil {
		return nil, fmt.Errorf("%s: webhook deliverer is required", op)
	}

	if clk == nil {
		return nil, fmt.Errorf("%s: clock is required", op)
	}

	if logger == nil {
		return nil, fmt.Errorf("%s: logger is required", op)
	}

	if businessMetrics == nil {
		return nil, fmt.Errorf("business metrics is required")
	}

	return &Dispatcher{
		pool:            pool,
		checkoutSession: checkoutSession,
		secrets:         secrets,
		deliverer:       deliverer,
		policy:          policy,
		clock:           clk,
		logger:          logger,
		businessMetrics: businessMetrics,
	}, nil
}

// Close releases no resources; the dispatcher does not own its collaborators.
func (d *Dispatcher) Close() error { return nil }

// Publish delivers msg to the merchant webhook named by destination.
//
// A nil result means the endpoint returned 2xx. Errors satisfying
// messaging.IsNonRetryable are permanent; all others are retried by the outbox.
func (d *Dispatcher) Publish(ctx context.Context, destination string, msg messaging.Message) error {
	const op = "merchantwebhookhttp.Dispatcher.Publish"

	logger := d.logger.With("destination", destination, "event_type", headerString(msg, "event_type"))

	merchantID, err := webhook.ParseMerchantDestination(destination)
	if err != nil {
		return d.permanent(ctx, logger, outcomeInvalidMessage, fmt.Errorf("%s: %w: %w", op, ErrInvalidMessage, err))
	}

	eventID, body, err := parseEnvelope(msg.Value)
	if err != nil {
		return d.permanent(ctx, logger, outcomeInvalidMessage, fmt.Errorf("%s: %w: %w", op, ErrInvalidMessage, err))
	}

	logger = logger.With("merchant_id", merchantID.String(), "event_id", eventID)

	// Resolve the current destination at delivery time. The connection is
	// released before any network I/O.
	var dest model.MerchantWebhookDestination

	err = d.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		sessionQ := d.checkoutSession.Conn(conn)

		found, findErr := sessionQ.FindWebhookDestination(ctx, merchantID)
		if findErr != nil {
			return findErr
		}

		dest = found

		return nil
	})
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return d.permanent(ctx, logger, outcomeDestinationMissing, fmt.Errorf("%s: %w", op, ErrDestinationMissing))
		}

		return d.retryable(ctx, logger, fmt.Errorf("%s: load webhook destination: %w", op, err))
	}

	if !dest.Enabled {
		return d.permanent(ctx, logger, outcomeDestinationDisabled, fmt.Errorf("%s: %w", op, ErrDestinationDisabled))
	}

	target, err := d.policy.ValidateURL(dest.WebhookURL)
	if err != nil {
		return d.permanent(ctx, logger, outcomeInvalidDestination, fmt.Errorf("%s: validate webhook url: %w", op, err))
	}

	if dest.SigningSecretCiphertext == "" {
		return d.permanent(ctx, logger, outcomeSigningUnavailable, fmt.Errorf("%s: %w", op, ErrSigningSecretMissing))
	}

	secret, err := d.secrets.Decrypt(dest.SigningSecretCiphertext, dest.SigningSecretKeyID)
	if err != nil {
		// Key material is operator-managed; a missing key may be restored while the
		// row still has retry budget, so this stays retryable.
		logger.Error("webhook signing secret could not be decrypted", "key_id", dest.SigningSecretKeyID)
		d.businessMetrics.WebhookDeliveryCompleted(ctx, outcomeSigningUnavailable)

		return fmt.Errorf("%s: decrypt signing secret: %w", op, err)
	}

	timestamp := d.clock.Now().Unix()

	headers := map[string]string{
		"Content-Type":          "application/json",
		"User-Agent":            "bnpl-webhook/1",
		webhook.HeaderID:        eventID,
		webhook.HeaderTimestamp: strconv.FormatInt(timestamp, 10),
		webhook.HeaderSignature: Sign([]byte(secret), eventID, timestamp, body),
	}

	if eventType := headerString(msg, "event_type"); eventType != "" {
		headers["X-BNPL-Webhook-Event"] = eventType
	}

	err = d.deliverer.Deliver(ctx, webhook.Delivery{URL: target, Headers: headers, Body: body})
	if err != nil {
		if messaging.IsNonRetryable(err) {
			return d.permanent(ctx, logger, outcomePermanentFailure, fmt.Errorf("%s: deliver webhook: %w", op, err))
		}

		d.businessMetrics.WebhookDeliveryCompleted(ctx, outcomeRetryableFailure)
		logger.Warn("webhook delivery failed; will retry if budget remains", "error", err.Error())

		return fmt.Errorf("%s: deliver webhook: %w", op, err)
	}

	d.businessMetrics.WebhookDeliveryCompleted(ctx, outcomeDelivered)
	logger.Info("webhook delivered")

	return nil
}

// permanent records an audit-friendly log line and metric, then returns err
// marked non-retryable.
func (d *Dispatcher) permanent(ctx context.Context, logger log.Logger, outcome string, err error) error {
	d.businessMetrics.WebhookDeliveryCompleted(ctx, outcome)
	logger.Warn("webhook undeliverable; no further attempts", "outcome", outcome, "error", err.Error())

	return messaging.NonRetryable(err)
}

// retryable records the failure and returns err unchanged so the outbox retries.
func (d *Dispatcher) retryable(ctx context.Context, logger log.Logger, err error) error {
	d.businessMetrics.WebhookDeliveryCompleted(ctx, outcomeRetryableFailure)
	logger.Warn("webhook delivery failed; will retry if budget remains", "error", err.Error())

	return err
}

// parseEnvelope extracts the stable event id and returns the compacted JSON body.
// The compacted bytes are both signed and sent, so the signature covers exactly
// what the receiver reads.
func parseEnvelope(payload []byte) (string, []byte, error) {
	var envelope struct {
		EventID string `json:"event_id"`
	}

	if err := json.Unmarshal(payload, &envelope); err != nil {
		return "", nil, errors.New("payload is not a json object")
	}

	eventID := strings.TrimSpace(envelope.EventID)
	if eventID == "" {
		return "", nil, errors.New("payload has no event_id")
	}

	if strings.ContainsAny(eventID, "\r\n") {
		return "", nil, errors.New("event_id contains control characters")
	}

	var compact bytes.Buffer
	if err := json.Compact(&compact, payload); err != nil {
		return "", nil, errors.New("compact payload")
	}

	return eventID, compact.Bytes(), nil
}

func headerString(msg messaging.Message, name string) string {
	return string(msg.Headers[name])
}
