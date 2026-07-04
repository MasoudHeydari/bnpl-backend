package bale

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	adapterbotmessage "github.com/parstis/bnpl-backend/pkg/adapter/external/botmessage"
	corebotmessage "github.com/parstis/bnpl-backend/pkg/core/external/botmessage"
	corehttpclient "github.com/parstis/bnpl-backend/pkg/core/httpclient"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
)

const (
	apiBaseURL       = "https://tapi.bale.ai"
	maxResponseBytes = 4 << 20
)

// Config contains Bale transport settings.
type Config struct {
	// Token is the Bale bot authentication token. It must never be logged.
	Token string

	// PollTimeout is the server-side long-poll wait requested from getUpdates.
	PollTimeout time.Duration

	// RetryBaseDelay is the initial delay after a transient polling failure.
	RetryBaseDelay time.Duration

	// RetryMaxDelay caps transient polling backoff.
	RetryMaxDelay time.Duration
}

// Client implements one Bale bot integration.
type Client struct {
	token          string
	pollTimeout    time.Duration
	retryBaseDelay time.Duration
	retryMaxDelay  time.Duration
	httpClient     corehttpclient.Client
	logger         corelog.Logger
}

var _ adapterbotmessage.Integration = (*Client)(nil)

// New constructs a Bale adapter without performing network I/O.
func New(cfg Config, httpClient corehttpclient.Client, logger corelog.Logger) (*Client, error) {
	const op = "bale.New"

	if strings.TrimSpace(cfg.Token) == "" {
		return nil, fmt.Errorf("%s: bale bot token is empty", op)
	}

	if httpClient == nil {
		return nil, fmt.Errorf("%s: bale http client is nil", op)
	}

	if logger == nil {
		return nil, fmt.Errorf("%s: bale logger is nil", op)
	}

	if cfg.PollTimeout < time.Second {
		return nil, fmt.Errorf("%s: bale poll timeout must be at least one second", op)
	}

	if cfg.RetryBaseDelay <= 0 {
		return nil, fmt.Errorf("%s: bale retry base delay must be positive", op)
	}

	if cfg.RetryMaxDelay < cfg.RetryBaseDelay {
		return nil, fmt.Errorf("%s: bale retry max delay must be >= retry base delay", op)
	}

	return &Client{
		token:          cfg.Token,
		pollTimeout:    cfg.PollTimeout,
		retryBaseDelay: cfg.RetryBaseDelay,
		retryMaxDelay:  cfg.RetryMaxDelay,
		httpClient:     httpClient,
		logger:         logger,
	}, nil
}

// ID returns the stable Bale integration routing identifier.
func (c *Client) ID() adapterbotmessage.IntegrationID {
	return adapterbotmessage.BaleIntegrationID
}

// Send delivers one text message through Bale.
func (c *Client) Send(ctx context.Context, input corebotmessage.SendInput) (corebotmessage.SendResult, error) {
	const op = "bale.Client.Send"

	if strings.TrimSpace(input.Destination.Address) == "" {
		return corebotmessage.SendResult{}, corebotmessage.PermanentError(errors.New("bale destination address is empty"))
	}

	if input.Text == "" {
		return corebotmessage.SendResult{}, corebotmessage.PermanentError(errors.New("bale message text is empty"))
	}

	payload := sendMessageRequest{
		ChatID: input.Destination.Address,
		Text:   input.Text,
	}
	if input.ReplyToExternalMessageID != "" {
		messageID, err := strconv.ParseInt(input.ReplyToExternalMessageID, 10, 64)
		if err != nil || messageID <= 0 {
			return corebotmessage.SendResult{}, corebotmessage.PermanentError(
				fmt.Errorf("%s: invalid bale reply message id %q", op, input.ReplyToExternalMessageID),
			)
		}

		payload.ReplyToMessageID = messageID
	}

	var message baleMessage
	err := c.callSend(ctx, "sendMessage", payload, &message)
	if err != nil {
		return corebotmessage.SendResult{}, err
	}

	if message.MessageID <= 0 {
		return corebotmessage.SendResult{}, corebotmessage.AmbiguousError(errors.New("bale sendMessage returned empty message id"))
	}

	return corebotmessage.SendResult{ExternalMessageID: strconv.FormatInt(message.MessageID, 10)}, nil
}

// Run receives Bale text messages with getUpdates long polling.
func (c *Client) Run(ctx context.Context, handler corebotmessage.IncomingMessageHandler) error {
	const op = "bale.Client.Run"

	if handler == nil {
		return fmt.Errorf("%s: bale incoming message handler is nil", op)
	}

	var offset int64
	pollBackoff := time.Duration(0)
	handlerBackoff := time.Duration(0)

	for {
		err := ctx.Err()
		if err != nil {
			return nil
		}

		updates, retryAfter, err := c.getUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, context.Canceled) {
				return nil
			}

			if isPermanentPollingError(err) {
				return err
			}

			pollBackoff = nextBackoff(pollBackoff, c.retryBaseDelay, c.retryMaxDelay)
			if retryAfter > pollBackoff {
				pollBackoff = retryAfter
			}

			c.logPollingFailure(err, pollBackoff)
			err = waitContext(ctx, pollBackoff)
			if err != nil {
				return nil
			}

			continue
		}

		pollBackoff = 0
		for _, update := range updates {
			if update.UpdateID <= 0 {
				return errors.New("bale update missing update_id")
			}

			incoming, ok := mapIncoming(update)
			if !ok {
				offset = update.UpdateID + 1
				continue
			}

			result, err := handler.Handle(ctx, incoming)
			if err != nil {
				// Keep offset unchanged so Bale can redeliver the update. A handler
				// error represents an application/internal failure, not an unsupported
				// user command; retry it without killing the process.
				handlerBackoff = nextBackoff(handlerBackoff, c.retryBaseDelay, c.retryMaxDelay)
				c.logHandlerFailure(incoming, err, handlerBackoff)
				err = waitContext(ctx, handlerBackoff)
				if err != nil {
					return nil
				}

				break
			}

			handlerBackoff = 0
			c.sendReplies(ctx, incoming, result)
			offset = update.UpdateID + 1
		}
	}
}

func (c *Client) getUpdates(ctx context.Context, offset int64) ([]baleUpdate, time.Duration, error) {
	payload := struct {
		Offset  int64 `json:"offset,omitempty"`
		Limit   int   `json:"limit"`
		Timeout int   `json:"timeout"`
	}{
		Offset:  offset,
		Limit:   100,
		Timeout: int(c.pollTimeout / time.Second),
	}

	var updates []baleUpdate
	retryAfter, err := c.callPolling(ctx, "getUpdates", payload, &updates)
	return updates, retryAfter, err
}

func (c *Client) sendReplies(
	ctx context.Context,
	incoming corebotmessage.IncomingMessage,
	result corebotmessage.HandleResult,
) {
	for _, reply := range result.Replies {
		input := corebotmessage.SendInput{
			Destination: incoming.Conversation,
			Text:        reply.Text,
		}
		if reply.ReplyToIncomingMessage {
			input.ReplyToExternalMessageID = incoming.ExternalMessageID
		}

		_, err := c.Send(ctx, input)
		if err != nil {
			c.logger.With(
				"integration",
				string(c.ID()),
				"external_event_id",
				incoming.ExternalEventID,
				"error",
				err.Error(),
			).Error("bot message reply delivery failed")
		}
	}
}

func (c *Client) callSend(ctx context.Context, method string, payload any, destination any) error {
	const op = "bale.Client.callSend"

	responseBody, statusCode, err := c.doJSON(ctx, method, payload)
	if err != nil {
		return classifyTransportSendError(err)
	}

	envelope := apiEnvelope{}
	err = json.Unmarshal(responseBody, &envelope)
	if err != nil {
		return corebotmessage.AmbiguousError(fmt.Errorf("%s: decode bale %s response: %w", op, method, err))
	}

	if !envelope.OK || statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		providerErr := &providerError{
			Code:        firstNonZero(envelope.ErrorCode, statusCode),
			Description: envelope.Description,
			RetryAfter:  envelope.Parameters.RetryAfter,
		}
		return classifyProviderSendError(providerErr)
	}

	if destination != nil {
		err = json.Unmarshal(envelope.Result, destination)
		if err != nil {
			return corebotmessage.AmbiguousError(fmt.Errorf("%s: decode bale %s result: %w", op, method, err))
		}
	}

	return nil
}

func (c *Client) callPolling(ctx context.Context, method string, payload any, destination any) (time.Duration, error) {
	const op = "bale.Client.callPolling"

	responseBody, statusCode, err := c.doJSON(ctx, method, payload)
	if err != nil {
		return 0, err
	}

	envelope := apiEnvelope{}
	err = json.Unmarshal(responseBody, &envelope)
	if err != nil {
		return 0, fmt.Errorf("%s: decode bale %s response: %w", op, method, err)
	}

	if !envelope.OK || statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		providerErr := &providerError{
			Code:        firstNonZero(envelope.ErrorCode, statusCode),
			Description: envelope.Description,
			RetryAfter:  envelope.Parameters.RetryAfter,
		}
		return time.Duration(providerErr.RetryAfter) * time.Second, providerErr
	}

	if destination != nil {
		err = json.Unmarshal(envelope.Result, destination)
		if err != nil {
			return 0, fmt.Errorf("%s: decode bale %s result: %w", op, method, err)
		}
	}

	return 0, nil
}

func (c *Client) doJSON(ctx context.Context, method string, payload any) ([]byte, int, error) {
	const op = "bale.Client.doJSON"

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: marshal bale %s request: %w", op, method, err)
	}

	url := apiBaseURL + "/bot" + c.token + "/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("%s: create bale %s request: %w", op, method, err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: execute bale %s request: %w", op, method, err)
	}

	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("%s: read bale %s response: %w", op, method, err)
	}

	if len(responseBody) > maxResponseBytes {
		return nil, resp.StatusCode, fmt.Errorf("%s: bale %s response exceeds %d bytes", op, method, maxResponseBytes)
	}

	return responseBody, resp.StatusCode, nil
}

func mapIncoming(update baleUpdate) (corebotmessage.IncomingMessage, bool) {
	if update.Message == nil || update.Message.Text == "" {
		return corebotmessage.IncomingMessage{}, false
	}

	kind, ok := mapDestinationKind(update.Message.Chat.Type)
	if !ok {
		return corebotmessage.IncomingMessage{}, false
	}

	var senderID string
	if update.Message.From != nil {
		senderID = strconv.FormatInt(update.Message.From.ID, 10)
	} else if update.Message.SenderChat != nil {
		senderID = strconv.FormatInt(update.Message.SenderChat.ID, 10)
	}

	integrationID := string(adapterbotmessage.BaleIntegrationID)
	return corebotmessage.IncomingMessage{
		IntegrationID:     integrationID,
		ExternalEventID:   strconv.FormatInt(update.UpdateID, 10),
		ExternalMessageID: strconv.FormatInt(update.Message.MessageID, 10),
		Conversation: corebotmessage.Destination{
			IntegrationID: integrationID,
			Address:       strconv.FormatInt(update.Message.Chat.ID, 10),
			Kind:          kind,
		},
		Sender:     corebotmessage.Participant{ID: senderID},
		Text:       update.Message.Text,
		OccurredAt: time.Unix(update.Message.Date, 0).UTC(),
	}, true
}

func mapDestinationKind(chatType string) (corebotmessage.DestinationKind, bool) {
	switch chatType {
	case "private":
		return corebotmessage.PrivateChatDestination, true

	case "group", "supergroup":
		return corebotmessage.GroupDestination, true

	case "channel":
		return corebotmessage.ChannelDestination, true

	default:
		return "", false
	}
}

type apiEnvelope struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	ErrorCode   int             `json:"error_code"`
	Description string          `json:"description"`
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

type sendMessageRequest struct {
	ChatID           string `json:"chat_id"`
	Text             string `json:"text"`
	ReplyToMessageID int64  `json:"reply_to_message_id,omitempty"`
}

type baleUpdate struct {
	UpdateID int64        `json:"update_id"`
	Message  *baleMessage `json:"message,omitempty"`
}

type baleMessage struct {
	MessageID  int64     `json:"message_id"`
	From       *baleUser `json:"from,omitempty"`
	Date       int64     `json:"date"`
	Chat       baleChat  `json:"chat"`
	SenderChat *baleChat `json:"sender_chat,omitempty"`
	Text       string    `json:"text,omitempty"`
}

type baleUser struct {
	ID int64 `json:"id"`
}

type baleChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type providerError struct {
	Code        int
	Description string
	RetryAfter  int
}

func (e *providerError) Error() string {
	return fmt.Sprintf("bale api error %d: %s", e.Code, e.Description)
}

func classifyTransportSendError(err error) error {
	if errors.Is(err, context.Canceled) {
		return err
	}

	if errors.Is(err, corehttpclient.ErrSaturated) || errors.Is(err, corehttpclient.ErrQueueWaitTimeout) {
		return corebotmessage.RetryableError(err)
	}

	if errors.Is(err, corehttpclient.ErrDisabled) || errors.Is(err, corehttpclient.ErrNilRequest) {
		return corebotmessage.PermanentError(err)
	}

	return corebotmessage.AmbiguousError(err)
}

func classifyProviderSendError(err *providerError) error {
	if err == nil {
		return nil
	}

	switch {
	case err.Code == http.StatusTooManyRequests:
		return corebotmessage.RetryableError(err)

	case err.Code >= http.StatusInternalServerError:
		return corebotmessage.RetryableError(err)

	case err.Code >= http.StatusBadRequest && err.Code < http.StatusInternalServerError:
		return corebotmessage.PermanentError(err)

	default:
		return corebotmessage.AmbiguousError(err)
	}
}

func isPermanentPollingError(err error) bool {
	var providerErr *providerError
	if errors.As(err, &providerErr) {
		switch providerErr.Code {
		case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict:
			return true
		}
	}

	return errors.Is(err, corehttpclient.ErrDisabled)
}

func firstNonZero(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}

	return 0
}

func nextBackoff(current, base, max time.Duration) time.Duration {
	if current <= 0 {
		return base
	}

	current *= 2
	if current > max {
		return max
	}

	return current
}

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()

	case <-timer.C:
		return nil
	}
}

func (c *Client) logHandlerFailure(
	incoming corebotmessage.IncomingMessage,
	err error,
	backoff time.Duration,
) {
	c.logger.With(
		"integration",
		string(c.ID()),
		"external_event_id",
		incoming.ExternalEventID,
		"backoff",
		backoff.String(),
		"error",
		err.Error(),
	).Error("bot message application handler failed; retrying update")
}

func (c *Client) logPollingFailure(err error, backoff time.Duration) {
	c.logger.With(
		"integration",
		string(c.ID()),
		"backoff",
		backoff.String(),
		"error",
		err.Error(),
	).Warn("bot message polling failed; retrying")
}
