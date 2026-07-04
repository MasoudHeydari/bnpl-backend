package telegram

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

	telegrambot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	adapterbotmessage "github.com/parstis/bnpl-backend/pkg/adapter/external/botmessage"
	corebotmessage "github.com/parstis/bnpl-backend/pkg/core/external/botmessage"
	corehttpclient "github.com/parstis/bnpl-backend/pkg/core/httpclient"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
)

const (
	apiBaseURL       = "https://api.telegram.org"
	maxResponseBytes = 4 << 20 // 4 MiB; getUpdates/sendMessage responses are expected to be small.
)

// Config contains Telegram transport settings.
type Config struct {
	// Token is the BotFather authentication token. It must never be logged.
	Token string

	// PollTimeout is the server-side long-poll wait requested from getUpdates.
	PollTimeout time.Duration

	// RetryBaseDelay is the initial delay after a transient polling failure.
	RetryBaseDelay time.Duration

	// RetryMaxDelay caps transient polling backoff.
	RetryMaxDelay time.Duration
}

// Client implements one Telegram bot integration.
type Client struct {
	token          string
	pollTimeout    time.Duration
	retryBaseDelay time.Duration
	retryMaxDelay  time.Duration
	httpClient     corehttpclient.Client
	bot            *telegrambot.Bot
	logger         corelog.Logger
}

var _ adapterbotmessage.Integration = (*Client)(nil)

// New constructs a Telegram adapter without performing network I/O.
//
// WithSkipGetMe is intentional: application construction should not become
// availability-coupled to Telegram. Runtime polling/send operations surface
// provider failures through the normal supervisor and Sender error semantics.
func New(cfg Config, httpClient corehttpclient.Client, logger corelog.Logger) (*Client, error) {
	const op = "telegram.New"

	if strings.TrimSpace(cfg.Token) == "" {
		return nil, fmt.Errorf("%s: telegram bot token is empty", op)
	}

	if httpClient == nil {
		return nil, fmt.Errorf("%s: telegram http client is nil", op)
	}

	if logger == nil {
		return nil, fmt.Errorf("%s: telegram logger is nil", op)
	}

	if cfg.PollTimeout < time.Second {
		return nil, fmt.Errorf("%s: telegram poll timeout must be at least one second", op)
	}

	if cfg.RetryBaseDelay <= 0 {
		return nil, fmt.Errorf("%s: telegram retry base delay must be positive", op)
	}

	if cfg.RetryMaxDelay < cfg.RetryBaseDelay {
		return nil, fmt.Errorf("%s: telegram retry max delay must be >= retry base delay", op)
	}

	b, err := telegrambot.New(
		cfg.Token,
		telegrambot.WithHTTPClient(cfg.PollTimeout, httpClient),
		telegrambot.WithSkipGetMe(),
	)
	if err != nil {
		return nil, fmt.Errorf("%s: create telegram bot client: %w", op, err)
	}

	return &Client{
		token:          cfg.Token,
		pollTimeout:    cfg.PollTimeout,
		retryBaseDelay: cfg.RetryBaseDelay,
		retryMaxDelay:  cfg.RetryMaxDelay,
		httpClient:     httpClient,
		bot:            b,
		logger:         logger,
	}, nil
}

// ID returns the stable Telegram integration routing identifier.
func (c *Client) ID() adapterbotmessage.IntegrationID {
	return adapterbotmessage.TelegramIntegrationID
}

// Send delivers one text message through Telegram.
func (c *Client) Send(ctx context.Context, input corebotmessage.SendInput) (corebotmessage.SendResult, error) {
	const op = "telegram.Client.Send"

	if strings.TrimSpace(input.Destination.Address) == "" {
		return corebotmessage.SendResult{}, corebotmessage.PermanentError(errors.New("telegram destination address is empty"))
	}

	if input.Text == "" {
		return corebotmessage.SendResult{}, corebotmessage.PermanentError(errors.New("telegram message text is empty"))
	}

	params := &telegrambot.SendMessageParams{
		ChatID: normalizeChatID(input.Destination.Address),
		Text:   input.Text,
	}

	if input.ReplyToExternalMessageID != "" {
		messageID, err := strconv.Atoi(input.ReplyToExternalMessageID)
		if err != nil || messageID <= 0 {
			return corebotmessage.SendResult{}, corebotmessage.PermanentError(
				fmt.Errorf("%s: invalid telegram reply message id %q", op, input.ReplyToExternalMessageID),
			)
		}

		params.ReplyParameters = &models.ReplyParameters{MessageID: messageID}
	}

	message, err := c.bot.SendMessage(ctx, params)
	if err != nil {
		return corebotmessage.SendResult{}, classifySendError(err)
	}

	if message == nil {
		return corebotmessage.SendResult{}, corebotmessage.AmbiguousError(errors.New("telegram sendMessage returned nil message"))
	}

	return corebotmessage.SendResult{ExternalMessageID: strconv.Itoa(message.ID)}, nil
}

// Run receives Telegram text messages with getUpdates long polling.
//
// Polling is implemented here rather than by Bot.Start so the next offset is
// advanced only after the application handler has completed. This preserves a
// clear acknowledgement boundary for business-facing inbound processing.
func (c *Client) Run(ctx context.Context, handler corebotmessage.IncomingMessageHandler) error {
	const op = "telegram.Client.Run"

	if handler == nil {
		return fmt.Errorf("%s: telegram incoming message handler is nil", op)
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
		for _, raw := range updates {
			updateID, update, decodeErr := decodeUpdate(raw)
			if updateID == 0 {
				return fmt.Errorf("%s: telegram update missing update_id: %w", op, decodeErr)
			}

			if decodeErr != nil {
				// A provider schema addition must not poison the entire update queue.
				// We can safely skip this item because update_id is still known.
				c.logSkippedUpdate(updateID, decodeErr)
				offset = updateID + 1
				continue
			}

			incoming, ok := mapIncoming(update)
			if !ok {
				offset = updateID + 1
				continue
			}

			result, err := handler.Handle(ctx, incoming)
			if err != nil {
				// Keep offset unchanged so Telegram can redeliver the update. A
				// handler error represents an application/internal failure, not an
				// unsupported user command; retry it without killing the process.
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
			offset = updateID + 1
		}
	}
}

func (c *Client) getUpdates(ctx context.Context, offset int64) ([]json.RawMessage, time.Duration, error) {
	const op = "telegram.Client.getUpdates"

	payload := struct {
		Offset         int64    `json:"offset,omitempty"`
		Limit          int      `json:"limit"`
		Timeout        int      `json:"timeout"`
		AllowedUpdates []string `json:"allowed_updates"`
	}{
		Offset:         offset,
		Limit:          100,
		Timeout:        int(c.pollTimeout / time.Second),
		AllowedUpdates: []string{models.AllowedUpdateMessage},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: marshal telegram getUpdates request: %w", op, err)
	}

	url := apiBaseURL + "/bot" + c.token + "/getUpdates"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("%s: create telegram getUpdates request: %w", op, err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: execute telegram getUpdates request: %w", op, err)
	}

	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, 0, fmt.Errorf("%s: read telegram getUpdates response: %w", op, err)
	}

	if len(responseBody) > maxResponseBytes {
		return nil, 0, fmt.Errorf("%s: telegram getUpdates response exceeds %d bytes", op, maxResponseBytes)
	}

	var envelope struct {
		OK          bool              `json:"ok"`
		Result      []json.RawMessage `json:"result"`
		ErrorCode   int               `json:"error_code"`
		Description string            `json:"description"`
		Parameters  struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
	}

	err = json.Unmarshal(responseBody, &envelope)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: decode telegram getUpdates response: %w", op, err)
	}

	if !envelope.OK {
		apiErr := &providerError{Code: envelope.ErrorCode, Description: envelope.Description}
		return nil, time.Duration(envelope.Parameters.RetryAfter) * time.Second, apiErr
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, 0, &providerError{Code: resp.StatusCode, Description: http.StatusText(resp.StatusCode)}
	}

	return envelope.Result, 0, nil
}

func (c *Client) sendReplies(ctx context.Context, incoming corebotmessage.IncomingMessage, result corebotmessage.HandleResult) {
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
			// Do not re-run the application handler because reply delivery failed:
			// the handler may already have committed business side effects. Durable
			// reply delivery can later be moved behind an outbox without changing
			// the core handler contract.
			c.logger.With(
				"integration", string(c.ID()),
				"external_event_id", incoming.ExternalEventID,
				"error", err.Error(),
			).Error("bot message reply delivery failed")
		}
	}
}

func mapIncoming(update *models.Update) (corebotmessage.IncomingMessage, bool) {
	if update == nil || update.Message == nil || update.Message.Text == "" {
		return corebotmessage.IncomingMessage{}, false
	}

	message := update.Message
	kind, ok := mapDestinationKind(message.Chat.Type)
	if !ok {
		return corebotmessage.IncomingMessage{}, false
	}

	var senderID string
	if message.From != nil {
		senderID = strconv.FormatInt(message.From.ID, 10)
	} else if message.SenderChat != nil {
		senderID = strconv.FormatInt(message.SenderChat.ID, 10)
	}

	integrationID := string(adapterbotmessage.TelegramIntegrationID)
	return corebotmessage.IncomingMessage{
		IntegrationID:     integrationID,
		ExternalEventID:   strconv.FormatInt(update.ID, 10),
		ExternalMessageID: strconv.Itoa(message.ID),
		Conversation: corebotmessage.Destination{
			IntegrationID: integrationID,
			Address:       strconv.FormatInt(message.Chat.ID, 10),
			Kind:          kind,
		},
		Sender:     corebotmessage.Participant{ID: senderID},
		Text:       message.Text,
		OccurredAt: time.Unix(int64(message.Date), 0).UTC(),
	}, true
}

func mapDestinationKind(chatType models.ChatType) (corebotmessage.DestinationKind, bool) {
	switch chatType {
	case models.ChatTypePrivate:
		return corebotmessage.PrivateChatDestination, true

	case models.ChatTypeGroup, models.ChatTypeSupergroup:
		return corebotmessage.GroupDestination, true

	case models.ChatTypeChannel:
		return corebotmessage.ChannelDestination, true

	default:
		return "", false
	}
}

func decodeUpdate(raw json.RawMessage) (int64, *models.Update, error) {
	var idOnly struct {
		ID int64 `json:"update_id"`
	}
	_ = json.Unmarshal(raw, &idOnly)

	update := &models.Update{}
	err := json.Unmarshal(raw, update)
	if err != nil {
		return idOnly.ID, nil, err
	}

	return update.ID, update, nil
}

func normalizeChatID(address string) any {
	id, err := strconv.ParseInt(address, 10, 64)
	if err == nil {
		return id
	}

	return address
}

func classifySendError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.Canceled) {
		return err
	}

	if errors.Is(err, corehttpclient.ErrSaturated) || errors.Is(err, corehttpclient.ErrQueueWaitTimeout) {
		return corebotmessage.RetryableError(err)
	}

	var tooMany *telegrambot.TooManyRequestsError
	if errors.As(err, &tooMany) {
		return corebotmessage.RetryableError(err)
	}

	if errors.Is(err, telegrambot.ErrorBadRequest) ||
		errors.Is(err, telegrambot.ErrorUnauthorized) ||
		errors.Is(err, telegrambot.ErrorForbidden) ||
		errors.Is(err, telegrambot.ErrorNotFound) ||
		errors.Is(err, telegrambot.ErrorConflict) ||
		errors.Is(err, corehttpclient.ErrDisabled) {
		return corebotmessage.PermanentError(err)
	}

	// Once SendMessage reaches the transport, a timeout/read failure can occur
	// after Telegram accepted the message. Treat unknown errors as ambiguous so
	// callers never blind-retry a user-visible message.
	return corebotmessage.AmbiguousError(err)
}

type providerError struct {
	Code        int
	Description string
}

func (e *providerError) Error() string {
	return fmt.Sprintf("telegram api error %d: %s", e.Code, e.Description)
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
		"integration", string(c.ID()),
		"external_event_id", incoming.ExternalEventID,
		"backoff", backoff.String(),
		"error", err.Error(),
	).Error("bot message application handler failed; retrying update")
}

func (c *Client) logPollingFailure(err error, backoff time.Duration) {
	c.logger.With(
		"integration", string(c.ID()),
		"backoff", backoff.String(),
		"error", err.Error(),
	).Warn("bot message polling failed; retrying")
}

func (c *Client) logSkippedUpdate(updateID int64, err error) {
	c.logger.With(
		"integration", string(c.ID()),
		"external_event_id", strconv.FormatInt(updateID, 10),
		"error", err.Error(),
	).Error("bot message update could not be decoded and was skipped")
}
