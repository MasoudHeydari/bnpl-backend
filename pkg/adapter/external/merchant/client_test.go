package merchant

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	cbgobreaker "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/circuitbreaker/gobreaker"
	cbnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/circuitbreaker/noop"
	"github.com/parstis/bnpl-backend/pkg/core/circuitbreaker"

	coremerchant "github.com/parstis/bnpl-backend/pkg/core/external/merchant"
	"github.com/parstis/bnpl-backend/pkg/core/httpclient"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/stretchr/testify/require"
)

type fakeProfile struct{}

func (fakeProfile) IsSecureHTTPProfile() {}

type fakeExecutor struct {
	respond func(*http.Request) (*http.Response, error)
}

func (f fakeExecutor) Do(r *http.Request) (*http.Response, error) { return f.respond(r) }
func (fakeExecutor) Close() error                                 { return nil }
func (fakeExecutor) NewSecureProfile(httpclient.SecureProfileConfig) (httpclient.SecureProfile, error) {
	return fakeProfile{}, nil
}
func (f fakeExecutor) DoWithSecureProfile(r *http.Request, _ httpclient.SecureProfile) (*http.Response, error) {
	return f.respond(r)
}

func TestInternalMerchantWire(t *testing.T) {
	requestID, err := model.ParseMerchantParticipationRequestID("0bb456eb-945a-401d-b5d7-d51a974a7426")
	require.NoError(t, err)
	var requests []string
	executor := fakeExecutor{respond: func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "Bearer synthetic-token", r.Header.Get("Authorization"))
		require.True(t, strings.HasPrefix(r.URL.Path, "/internal/v1/business-lines/bnpl/requests/"+requestID.String()))
		if r.Method == http.MethodGet {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":{"merchant_id":"c4f139c9-8a44-42dd-9aad-6a0f60200b6d","request_id":"0bb456eb-945a-401d-b5d7-d51a974a7426","business_line":"bnpl","environment":"SANDBOX","trade_name":"Test","context_id":null,"commercial_status":"inactive","base_application_status":"APPROVED"},"meta":{"request_id":"http-only"}}`))}, nil
		}

		require.Equal(t, http.MethodPut, r.Method)
		require.Equal(t, "/internal/v1/business-lines/bnpl/requests/"+requestID.String()+"/progress", r.URL.Path)
		body, e := io.ReadAll(r.Body)
		require.NoError(t, e)
		requests = append(requests, string(body))
		var v map[string]any
		require.NoError(t, json.Unmarshal(body, &v))
		require.NotContains(t, v, "request_id")
		require.Equal(t, "READY", v["status"])
		require.Equal(t, []any{}, v["issues"])
		status := 200
		if len(requests) == 1 {
			status = 503
		}

		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"data":{"request_id":"0bb456eb-945a-401d-b5d7-d51a974a7426","code":"bnpl","environment":"SANDBOX","status":"READY","owner_version":1,"external_account_reference":"account"}}`))}, nil
	}}
	c, err := New(executor, cbnoop.New(), "https://merchant.internal", model.NewSecret("synthetic-token"))
	require.NoError(t, err)
	ref, err := c.GetParticipation(context.Background(), requestID)
	require.NoError(t, err)
	require.Equal(t, requestID, ref.RequestID)
	require.Equal(t, "Test", ref.TradeName)
	report := coremerchant.Progress{RequestID: requestID, EventID: "30dfce08-466e-4fe1-9c10-d170483b6fa2", OwnerVersion: 1, AccountReference: "account"}
	_, err = c.ReportReady(context.Background(), report)
	require.Error(t, err)
	require.NotErrorIs(t, err, coremerchant.ErrBlocked)
	snap, err := c.ReportReady(context.Background(), report)
	require.NoError(t, err)
	require.Equal(t, "account", snap.AccountReference)
	require.Equal(t, requests[0], requests[1])
}

func TestMerchantCircuitBreaker(t *testing.T) {
	requestID, err := model.ParseMerchantParticipationRequestID("0bb456eb-945a-401d-b5d7-d51a974a7426")
	require.NoError(t, err)
	calls := 0
	status := http.StatusConflict
	executor := fakeExecutor{respond: func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	}}
	breaker := cbgobreaker.New(cbgobreaker.Config{
		MaxRequests:      1,
		Timeout:          time.Minute,
		Interval:         time.Minute,
		FailureThreshold: 1,
	})
	c, err := New(executor, breaker, "https://merchant.internal", model.NewSecret("synthetic-token"))
	require.NoError(t, err)
	report := coremerchant.Progress{RequestID: requestID, EventID: "30dfce08-466e-4fe1-9c10-d170483b6fa2", OwnerVersion: 1, AccountReference: "account"}
	_, err = c.ReportReady(context.Background(), report)
	require.ErrorIs(t, err, coremerchant.ErrBlocked)
	status = http.StatusNotFound
	_, err = c.GetParticipation(context.Background(), requestID)
	require.ErrorIs(t, err, coremerchant.ErrReferenceNotFound)
	require.Equal(t, 2, calls)
	status = http.StatusServiceUnavailable
	_, err = c.GetParticipation(context.Background(), requestID)
	require.Error(t, err)
	require.NotErrorIs(t, err, circuitbreaker.ErrOpen)
	require.Equal(t, 3, calls)
	_, err = c.ReportReady(context.Background(), report)
	require.ErrorIs(t, err, circuitbreaker.ErrOpen)
	require.NotErrorIs(t, err, coremerchant.ErrBlocked)
	require.Equal(t, 3, calls, "open Merchant circuit must not call HTTP")
}

func TestMerchantClientRequiresBreakerAndBearerToken(t *testing.T) {
	executor := fakeExecutor{}
	for _, token := range []string{"", "invalid\r\ntoken", "two words"} {
		_, err := New(executor, cbnoop.New(), "https://merchant.internal", model.NewSecret(token))
		require.Error(t, err)
	}

	_, err := New(executor, nil, "https://merchant.internal", model.NewSecret("synthetic-token"))
	require.Error(t, err)
}
