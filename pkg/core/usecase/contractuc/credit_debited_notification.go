package contractuc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/notificationsms"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// enqueueCreditDebitedNotification persists a credit-debited Notification
// command in the same database transaction that recorded a positive exposure
// increase. When the feature flag is disabled, this is a no-op.
//
// Missing checkout session or external customer id fails closed so activation
// cannot commit without an observable, retryable notification deficit.
func (uc *UseCase) enqueueCreditDebitedNotification(
	ctx context.Context,
	tx repo.Tx,
	contractRow model.Contract,
	account model.CreditAccount,
	creditTransaction model.CreditTransaction,
	debitedAmountMinor uint64,
	debitOccurredAt time.Time,
) error {
	const op = "contractuc.UseCase.enqueueCreditDebitedNotification"

	if !uc.creditDebitedNotificationEnabled {
		return nil
	}

	if debitedAmountMinor == 0 {
		return nil
	}

	if creditTransaction.ID.IsZero() {
		return fmt.Errorf("%s: credit transaction id is required", op)
	}

	notificationType, err := creditDebitedNotificationType(account.CreditKind)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	currency, err := creditDebitedNotificationCurrency(account.Currency)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	checkoutSessionQ := uc.checkoutSession.Tx(tx)
	session, err := checkoutSessionQ.GetByContractID(ctx, contractRow.ID)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return fmt.Errorf(
				"%s: checkout session is required for credit-debited notification on contract %s",
				op,
				contractRow.ID,
			)
		}

		return fmt.Errorf("%s: load checkout session: %w", op, err)
	}

	if session.ID.IsZero() {
		return fmt.Errorf("%s: checkout session id is required", op)
	}

	customerProfileQ := uc.customerProfile.Tx(tx)
	profile, err := customerProfileQ.LockByID(ctx, contractRow.CustomerID)
	if err != nil {
		return fmt.Errorf("%s: load customer profile: %w", op, err)
	}

	if profile.ExternalCustomerID.IsZero() {
		return fmt.Errorf(
			"%s: external customer id is required for credit-debited notification on customer %s",
			op,
			contractRow.CustomerID,
		)
	}

	sourceEventID := creditTransaction.ID.String()
	expiresAt := debitOccurredAt.UTC().Add(notificationsms.CreditDebitedExpiryWindow)

	event := notificationsms.NewCreditDebitedCommand(notificationsms.CreditDebitedCommandInput{
		SourceEventID:            sourceEventID,
		IdempotencyKey:           notificationsms.CreditDebitedIdempotencyKey(sourceEventID),
		NotificationType:         notificationType,
		ExternalCustomerID:       profile.ExternalCustomerID.String(),
		CheckoutSessionID:        session.ID.String(),
		DebitedAmountMinor:       debitedAmountMinor,
		RemainingAvailableCredit: account.AvailableAmountMinor(),
		Currency:                 currency,
		ExpiresAt:                expiresAt,
	})
	if validateErr := notificationsms.ValidateCreditDebitedCommand(event); validateErr != nil {
		return fmt.Errorf("%s: validate notification command: %w", op, validateErr)
	}

	payload, marshalErr := json.Marshal(event)
	if marshalErr != nil {
		return fmt.Errorf("%s: marshal notification command: %w", op, marshalErr)
	}

	aggregateType := "credit_transaction"
	outboxQ := uc.outbox.Tx(tx)
	_, outboxErr := outboxQ.Create(ctx, repo.CreateOutboxMessage{
		Destination:   uc.notificationCommandsTopic,
		MessageKey:    profile.ExternalCustomerID.String(),
		EventType:     notificationsms.EventTypeCommand,
		AggregateType: &aggregateType,
		Payload:       payload,
		CreatedAt:     debitOccurredAt.UTC(),
	})
	if outboxErr != nil {
		return fmt.Errorf("%s: insert notification outbox: %w", op, outboxErr)
	}

	return nil
}

func creditDebitedNotificationType(kind model.CreditKind) (string, error) {
	switch kind {
	case model.PayNextMonthCreditKind:
		return notificationsms.NotificationTypeCreditMonthlyDebited, nil

	case model.PayInInstallmentsCreditKind:
		return notificationsms.NotificationTypeCreditInstallmentDebited, nil

	default:
		return "", fmt.Errorf("unsupported credit kind %q", kind)
	}
}

func creditDebitedNotificationCurrency(currency model.Currency) (string, error) {
	switch currency {
	case model.IRRCurrency:
		return "IRR", nil

	default:
		return "", fmt.Errorf("unsupported notification currency %q", currency)
	}
}
