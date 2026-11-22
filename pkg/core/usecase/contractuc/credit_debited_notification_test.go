package contractuc

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/notificationsms"
	coremock "github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

func TestEnqueueCreditDebitedNotification_FeatureFlagOffSkipsOutbox(t *testing.T) {
	t.Parallel()

	uc := &UseCase{creditDebitedNotificationEnabled: false}
	err := uc.enqueueCreditDebitedNotification(
		context.Background(),
		coremock.NewTx(t),
		model.Contract{ID: model.ContractID(uuid.New())},
		model.CreditAccount{CreditKind: model.PayNextMonthCreditKind},
		model.CreditTransaction{ID: model.CreditTransactionID(uuid.New()), AmountMinor: 100},
		100,
		time.Now().UTC(),
	)
	require.NoError(t, err)
}

func TestEnqueueCreditDebitedNotification_ZeroDebitSkipsOutbox(t *testing.T) {
	t.Parallel()

	uc := &UseCase{
		creditDebitedNotificationEnabled: true,
		notificationCommandsTopic:        "notification.commands.v1",
	}
	err := uc.enqueueCreditDebitedNotification(
		context.Background(),
		coremock.NewTx(t),
		model.Contract{ID: model.ContractID(uuid.New())},
		model.CreditAccount{CreditKind: model.PayNextMonthCreditKind},
		model.CreditTransaction{ID: model.CreditTransactionID(uuid.New())},
		0,
		time.Now().UTC(),
	)
	require.NoError(t, err)
}

func TestEnqueueCreditDebitedNotification_MonthlyAndInstallmentTemplates(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		kind         model.CreditKind
		wantTemplate string
	}{
		{
			name:         "monthly",
			kind:         model.PayNextMonthCreditKind,
			wantTemplate: notificationsms.NotificationTypeCreditMonthlyDebited,
		},
		{
			name:         "installment",
			kind:         model.PayInInstallmentsCreditKind,
			wantTemplate: notificationsms.NotificationTypeCreditInstallmentDebited,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			tx := coremock.NewTx(t)
			checkoutSession := coremock.NewCheckoutSession(t)
			checkoutSessionTx := coremock.NewCheckoutSessionTxQueryer(t)
			customerProfile := coremock.NewCustomerProfile(t)
			customerProfileTx := coremock.NewCustomerProfileTxQueryer(t)
			outbox := coremock.NewOutbox(t)
			outboxTx := coremock.NewOutboxTxQueryer(t)

			contractID := model.ContractID(uuid.New())
			customerID := model.CustomerID(uuid.New())
			sessionID := model.CheckoutSessionID(uuid.New())
			externalCustomerID := model.ExternalCustomerID(uuid.New())
			creditTxID := model.CreditTransactionID(uuid.New())
			debitAt := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

			checkoutSession.EXPECT().Tx(tx).Return(checkoutSessionTx).Once()
			checkoutSessionTx.EXPECT().GetByContractID(ctx, contractID).Return(model.CheckoutSession{
				ID: sessionID,
			}, nil).Once()

			customerProfile.EXPECT().Tx(tx).Return(customerProfileTx).Once()
			customerProfileTx.EXPECT().LockByID(ctx, customerID).Return(model.CustomerProfile{
				ID:                 customerID,
				ExternalCustomerID: externalCustomerID,
			}, nil).Once()

			outbox.EXPECT().Tx(tx).Return(outboxTx).Once()
			outboxTx.EXPECT().Create(ctx, mock.Anything).RunAndReturn(
				func(_ context.Context, msg repo.CreateOutboxMessage) (model.OutboxMessage, error) {
					require.Equal(t, "notification.commands.v1", msg.Destination)
					require.Equal(t, notificationsms.EventTypeCommand, msg.EventType)
					require.Equal(t, externalCustomerID.String(), msg.MessageKey)
					require.NotContains(t, string(msg.Payload), "x-notification-caller")
					require.NotContains(t, string(msg.Payload), "payment_url")
					require.NotContains(t, string(msg.Payload), `"phone"`)

					var event notificationsms.Command
					require.NoError(t, json.Unmarshal(msg.Payload, &event))
					require.NoError(t, notificationsms.ValidateCreditDebitedCommand(event))
					require.Equal(t, creditTxID.String(), event.SourceEventID)
					require.Equal(t, notificationsms.CreditDebitedIdempotencyKey(creditTxID.String()), event.IdempotencyKey)
					require.Equal(t, tc.wantTemplate, event.NotificationType)
					require.Equal(t, tc.wantTemplate, event.Targets[0].Deliveries[0].TemplateCode)
					require.Equal(t, externalCustomerID.String(), event.Targets[0].ExternalCustomerID)
					require.Equal(t, "BNPL_PURCHASE", event.Targets[0].BusinessReferenceType)
					require.Equal(t, "cs_"+sessionID.String(), event.Targets[0].BusinessReferenceID)
					require.Equal(t, "500000", event.Targets[0].Deliveries[0].Variables["debited_amount"])
					require.Equal(t, "3000000", event.Targets[0].Deliveries[0].Variables["remaining_available_credit"])
					require.Equal(t, "IRR", event.Targets[0].Deliveries[0].Variables["currency"])
					require.Equal(t, debitAt.Add(24*time.Hour), event.ExpiresAt)
					require.Equal(t, time.UTC, event.ExpiresAt.Location())

					return model.OutboxMessage{}, nil
				},
			).Once()

			uc := &UseCase{
				checkoutSession:                  checkoutSession,
				customerProfile:                  customerProfile,
				outbox:                           outbox,
				notificationCommandsTopic:        "notification.commands.v1",
				creditDebitedNotificationEnabled: true,
			}

			err := uc.enqueueCreditDebitedNotification(
				ctx,
				tx,
				model.Contract{ID: contractID, CustomerID: customerID},
				model.CreditAccount{
					CreditKind:          tc.kind,
					Currency:            model.IRRCurrency,
					LimitAmountMinor:    3_500_000,
					ExposureAmountMinor: 500_000,
				},
				model.CreditTransaction{ID: creditTxID, AmountMinor: 500_000},
				500_000,
				debitAt,
			)
			require.NoError(t, err)
		})
	}
}

func TestEnqueueCreditDebitedNotification_CreditKindsDoNotShareRemaining(t *testing.T) {
	t.Parallel()

	monthlyAvailable := uint64(1_000_000)
	installmentAvailable := uint64(9_000_000)

	monthlyAccount := model.CreditAccount{
		CreditKind:          model.PayNextMonthCreditKind,
		Currency:            model.IRRCurrency,
		LimitAmountMinor:    monthlyAvailable + 100,
		ExposureAmountMinor: 100,
	}
	installmentAccount := model.CreditAccount{
		CreditKind:          model.PayInInstallmentsCreditKind,
		Currency:            model.IRRCurrency,
		LimitAmountMinor:    installmentAvailable + 100,
		ExposureAmountMinor: 100,
	}

	require.Equal(t, monthlyAvailable, monthlyAccount.AvailableAmountMinor())
	require.Equal(t, installmentAvailable, installmentAccount.AvailableAmountMinor())
	require.NotEqual(t, monthlyAccount.AvailableAmountMinor(), installmentAccount.AvailableAmountMinor())
}

func TestEnqueueCreditDebitedNotification_MissingCheckoutSessionFailsClosed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tx := coremock.NewTx(t)
	checkoutSession := coremock.NewCheckoutSession(t)
	checkoutSessionTx := coremock.NewCheckoutSessionTxQueryer(t)

	contractID := model.ContractID(uuid.New())
	checkoutSession.EXPECT().Tx(tx).Return(checkoutSessionTx).Once()
	checkoutSessionTx.EXPECT().GetByContractID(ctx, contractID).
		Return(model.CheckoutSession{}, cerr.ResourceNotFound(errors.New("missing"))).Once()

	uc := &UseCase{
		checkoutSession:                  checkoutSession,
		notificationCommandsTopic:        "notification.commands.v1",
		creditDebitedNotificationEnabled: true,
	}

	err := uc.enqueueCreditDebitedNotification(
		ctx,
		tx,
		model.Contract{ID: contractID, CustomerID: model.CustomerID(uuid.New())},
		model.CreditAccount{
			CreditKind:          model.PayNextMonthCreditKind,
			Currency:            model.IRRCurrency,
			LimitAmountMinor:    1000,
			ExposureAmountMinor: 100,
		},
		model.CreditTransaction{ID: model.CreditTransactionID(uuid.New()), AmountMinor: 100},
		100,
		time.Now().UTC(),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "checkout session is required")
}

func TestEnqueueCreditDebitedNotification_MissingExternalCustomerIDFailsClosed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tx := coremock.NewTx(t)
	checkoutSession := coremock.NewCheckoutSession(t)
	checkoutSessionTx := coremock.NewCheckoutSessionTxQueryer(t)
	customerProfile := coremock.NewCustomerProfile(t)
	customerProfileTx := coremock.NewCustomerProfileTxQueryer(t)

	contractID := model.ContractID(uuid.New())
	customerID := model.CustomerID(uuid.New())

	checkoutSession.EXPECT().Tx(tx).Return(checkoutSessionTx).Once()
	checkoutSessionTx.EXPECT().GetByContractID(ctx, contractID).Return(model.CheckoutSession{
		ID: model.CheckoutSessionID(uuid.New()),
	}, nil).Once()

	customerProfile.EXPECT().Tx(tx).Return(customerProfileTx).Once()
	customerProfileTx.EXPECT().LockByID(ctx, customerID).Return(model.CustomerProfile{
		ID: customerID,
	}, nil).Once()

	uc := &UseCase{
		checkoutSession:                  checkoutSession,
		customerProfile:                  customerProfile,
		notificationCommandsTopic:        "notification.commands.v1",
		creditDebitedNotificationEnabled: true,
	}

	err := uc.enqueueCreditDebitedNotification(
		ctx,
		tx,
		model.Contract{ID: contractID, CustomerID: customerID},
		model.CreditAccount{
			CreditKind:          model.PayInInstallmentsCreditKind,
			Currency:            model.IRRCurrency,
			LimitAmountMinor:    1000,
			ExposureAmountMinor: 100,
		},
		model.CreditTransaction{ID: model.CreditTransactionID(uuid.New()), AmountMinor: 100},
		100,
		time.Now().UTC(),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "external customer id is required")
}

func TestCreditDebitedNotificationTypeMapping(t *testing.T) {
	t.Parallel()

	monthly, err := creditDebitedNotificationType(model.PayNextMonthCreditKind)
	require.NoError(t, err)
	require.Equal(t, notificationsms.NotificationTypeCreditMonthlyDebited, monthly)

	installment, err := creditDebitedNotificationType(model.PayInInstallmentsCreditKind)
	require.NoError(t, err)
	require.Equal(t, notificationsms.NotificationTypeCreditInstallmentDebited, installment)

	_, err = creditDebitedNotificationType("")
	require.Error(t, err)
}
