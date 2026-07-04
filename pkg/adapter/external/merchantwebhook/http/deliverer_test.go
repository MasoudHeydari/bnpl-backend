package http_test

import (
	"context"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	merchantwebhookhttp "github.com/parstis/bnpl-backend/pkg/adapter/external/merchantwebhook/http"
	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	httpstandard "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/httpclient/standard"
	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	metricsnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics/noop"
	corehttpclient "github.com/parstis/bnpl-backend/pkg/core/httpclient"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
	webhookpolicy "github.com/parstis/bnpl-backend/pkg/core/policy/webhook"
	"github.com/parstis/bnpl-backend/pkg/core/webhook"
)

type fakeResolver struct {
	addrs []netip.Addr
}

func (r fakeResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return r.addrs, nil
}

var devPolicy = webhookpolicy.Development()

func testHTTPClient(t *testing.T) *httpstandard.Client {
	t.Helper()

	client, err := httpstandard.New(
		httpstandard.Config{
			MaxInFlight:           10,
			MaxQueueSize:          10,
			QueueWaitTimeout:      time.Second,
			Timeout:               5 * time.Second,
			DialTimeout:           time.Second,
			TLSHandshakeTimeout:   time.Second,
			ResponseHeaderTimeout: time.Second,
			IdleConnTimeout:       time.Minute,
			MaxIdleConns:          10,
			MaxIdleConnsPerHost:   5,
			MaxConnsPerHost:       5,
		},
		adapterclock.NewSystem(time.UTC, time.UTC),
		lognoop.New(),
		metricsnoop.New(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	return client
}

func webhookProfile(
	t *testing.T,
	client *httpstandard.Client,
	policy webhookpolicy.Policy,
) corehttpclient.SecureProfile {
	t.Helper()

	profile, err := client.NewSecureProfile(corehttpclient.SecureProfileConfig{
		Name:                  "merchant_webhook",
		NetworkPolicy:         policy,
		Timeout:               merchantwebhookhttp.DefaultTimeout,
		DialTimeout:           merchantwebhookhttp.DefaultDialTimeout,
		TLSHandshakeTimeout:   merchantwebhookhttp.DefaultDialTimeout,
		ResponseHeaderTimeout: merchantwebhookhttp.DefaultTimeout,
		DisableKeepAlives:     true,
	})
	require.NoError(t, err)

	return profile
}

func newDeliverer(t *testing.T, cfg merchantwebhookhttp.Config) *merchantwebhookhttp.Deliverer {
	t.Helper()

	if cfg.Executor == nil {
		cfg.Executor = testHTTPClient(t)
	}

	d, err := merchantwebhookhttp.New(cfg)
	require.NoError(t, err)

	return d
}

func deliveryTo(t *testing.T, raw string) webhook.Delivery {
	t.Helper()

	u, err := url.Parse(raw)
	require.NoError(t, err)

	return webhook.Delivery{
		URL:     u,
		Headers: map[string]string{"Content-Type": "application/json", webhook.HeaderID: "evt-1"},
		Body:    []byte(`{"event_id":"evt-1"}`),
	}
}

func TestNewRequiresExecutor(t *testing.T) {
	t.Parallel()

	_, err := merchantwebhookhttp.New(merchantwebhookhttp.Config{Policy: devPolicy})
	require.Error(t, err)
	require.Contains(t, err.Error(), "executor is required")
}

func TestDeliverStatusClassification(t *testing.T) {
	t.Parallel()

	cases := []struct {
		status    int
		wantErr   bool
		permanent bool
	}{
		{status: 200}, {status: 202}, {status: 204},
		{status: 408, wantErr: true}, {status: 429, wantErr: true},
		{status: 500, wantErr: true}, {status: 503, wantErr: true},
		{status: 400, wantErr: true, permanent: true},
		{status: 401, wantErr: true, permanent: true},
		{status: 404, wantErr: true, permanent: true},
		{status: 410, wantErr: true, permanent: true},
		{status: 301, wantErr: true, permanent: true},
		{status: 302, wantErr: true, permanent: true},
	}

	for _, tc := range cases {
		t.Run(nethttp.StatusText(tc.status), func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, _ *nethttp.Request) {
				w.Header().Set("Location", "http://169.254.169.254/latest")
				w.WriteHeader(tc.status)
			}))
			t.Cleanup(srv.Close)

			d := newDeliverer(t, merchantwebhookhttp.Config{Policy: devPolicy})
			err := d.Deliver(context.Background(), deliveryTo(t, srv.URL))

			if !tc.wantErr {
				require.NoError(t, err)

				return
			}

			require.Error(t, err)
			require.Equal(t, tc.permanent, messaging.IsNonRetryable(err), err.Error())
			require.NotContains(t, err.Error(), srv.URL)
		})
	}
}

func TestDeliverSendsPostWithSignedHeadersAndBody(t *testing.T) {
	t.Parallel()

	var gotMethod, gotID, gotBody string

	srv := httptest.NewServer(nethttp.HandlerFunc(func(_ nethttp.ResponseWriter, r *nethttp.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotMethod, gotID, gotBody = r.Method, r.Header.Get(webhook.HeaderID), string(raw)
	}))
	t.Cleanup(srv.Close)

	d := newDeliverer(t, merchantwebhookhttp.Config{Policy: devPolicy})
	require.NoError(t, d.Deliver(context.Background(), deliveryTo(t, srv.URL)))

	require.Equal(t, nethttp.MethodPost, gotMethod)
	require.Equal(t, "evt-1", gotID)
	require.JSONEq(t, `{"event_id":"evt-1"}`, gotBody)
}

func TestDeliverDoesNotFollowRedirects(t *testing.T) {
	t.Parallel()

	var targetHits atomic.Int32

	target := httptest.NewServer(nethttp.HandlerFunc(func(nethttp.ResponseWriter, *nethttp.Request) {
		targetHits.Add(1)
	}))
	t.Cleanup(target.Close)

	srv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		nethttp.Redirect(w, r, target.URL, nethttp.StatusTemporaryRedirect)
	}))
	t.Cleanup(srv.Close)

	d := newDeliverer(t, merchantwebhookhttp.Config{Policy: devPolicy})
	err := d.Deliver(context.Background(), deliveryTo(t, srv.URL))

	require.Error(t, err)
	require.True(t, messaging.IsNonRetryable(err))
	require.Zero(t, targetHits.Load())
}

func TestDeliverTimeoutIsRetryable(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	srv := httptest.NewServer(nethttp.HandlerFunc(func(nethttp.ResponseWriter, *nethttp.Request) {
		select {
		case <-release:
		case <-time.After(5 * time.Second):
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	d := newDeliverer(t, merchantwebhookhttp.Config{Policy: devPolicy, Timeout: 100 * time.Millisecond})
	err := d.Deliver(context.Background(), deliveryTo(t, srv.URL))

	require.Error(t, err)
	require.False(t, messaging.IsNonRetryable(err))
	require.ErrorContains(t, err, "timed out")
}

func TestDeliverWebhookTimeoutIndependentOfDefaultClient(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	srv := httptest.NewServer(nethttp.HandlerFunc(func(nethttp.ResponseWriter, *nethttp.Request) {
		select {
		case <-release:
		case <-time.After(2 * time.Second):
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	client := testHTTPClient(t)
	d := newDeliverer(t, merchantwebhookhttp.Config{
		Policy:   devPolicy,
		Timeout:  150 * time.Millisecond,
		Executor: client,
	})

	started := time.Now()
	err := d.Deliver(context.Background(), deliveryTo(t, srv.URL))
	elapsed := time.Since(started)

	require.Error(t, err)
	require.False(t, messaging.IsNonRetryable(err))
	require.Less(t, elapsed, time.Second)
}

func TestDeliverConnectionRefusedIsRetryable(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(nethttp.HandlerFunc(func(nethttp.ResponseWriter, *nethttp.Request) {}))
	raw := srv.URL
	srv.Close()

	d := newDeliverer(t, merchantwebhookhttp.Config{Policy: devPolicy})
	err := d.Deliver(context.Background(), deliveryTo(t, raw))

	require.Error(t, err)
	require.False(t, messaging.IsNonRetryable(err))
}

func TestDeliverBoundsResponseBody(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, _ *nethttp.Request) {
		chunk := []byte(strings.Repeat("a", 1024))
		for i := 0; i < 1024; i++ {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)

	d := newDeliverer(t, merchantwebhookhttp.Config{Policy: devPolicy, MaxResponseBytes: 4096})
	require.NoError(t, d.Deliver(context.Background(), deliveryTo(t, srv.URL)))
}

func TestDeliverStrictPolicyBlocksLoopbackAtDialTime(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32

	srv := httptest.NewServer(nethttp.HandlerFunc(func(nethttp.ResponseWriter, *nethttp.Request) { hits.Add(1) }))
	t.Cleanup(srv.Close)

	d := newDeliverer(t, merchantwebhookhttp.Config{Policy: webhookpolicy.Policy{RequireHTTPS: false}})
	err := d.Deliver(context.Background(), deliveryTo(t, srv.URL))

	require.Error(t, err)
	require.True(t, messaging.IsNonRetryable(err))
	require.ErrorIs(t, err, webhookpolicy.ErrBlockedAddress)
	require.Zero(t, hits.Load())
}

func TestDeliverHostnameResolvingToBlockedAddressIsRejected(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32

	srv := httptest.NewServer(nethttp.HandlerFunc(func(nethttp.ResponseWriter, *nethttp.Request) { hits.Add(1) }))
	t.Cleanup(srv.Close)

	port := strings.TrimPrefix(srv.URL, "http://127.0.0.1")

	cases := map[string][]netip.Addr{
		"loopback":          {netip.MustParseAddr("127.0.0.1")},
		"private":           {netip.MustParseAddr("10.1.2.3")},
		"metadata":          {netip.MustParseAddr("169.254.169.254")},
		"mapped loopback":   {netip.MustParseAddr("::ffff:127.0.0.1")},
		"mixed public/priv": {netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("127.0.0.1")},
	}

	for name, addrs := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := testHTTPClient(t)
			policy := webhookpolicy.Strict()
			d := newDeliverer(t, merchantwebhookhttp.Config{
				Policy:   policy,
				Executor: client,
			})

			require.NoError(t, client.SetSecureProfileResolver(
				webhookProfile(t, client, policy),
				fakeResolver{addrs: addrs},
			))

			err := d.Deliver(context.Background(), deliveryTo(t, "http://rebind.example.com"+port))

			require.Error(t, err)
			require.True(t, messaging.IsNonRetryable(err), err.Error())
			require.ErrorIs(t, err, webhookpolicy.ErrBlockedAddress)
		})
	}

	require.Zero(t, hits.Load())
}

func TestDeliverDialsResolvedIPNotHostname(t *testing.T) {
	t.Parallel()

	var gotHost string

	srv := httptest.NewServer(nethttp.HandlerFunc(func(_ nethttp.ResponseWriter, r *nethttp.Request) { gotHost = r.Host }))
	t.Cleanup(srv.Close)

	port := strings.TrimPrefix(srv.URL, "http://127.0.0.1")

	client := testHTTPClient(t)
	d := newDeliverer(t, merchantwebhookhttp.Config{
		Policy:   devPolicy,
		Executor: client,
	})

	require.NoError(t, client.SetSecureProfileResolver(
		webhookProfile(t, client, devPolicy),
		fakeResolver{addrs: []netip.Addr{netip.MustParseAddr("127.0.0.1")}},
	))

	require.NoError(t, d.Deliver(context.Background(), deliveryTo(t, "http://merchant.example.com"+port)))
	require.Equal(t, "merchant.example.com"+port, gotHost)
}

func TestNewRejectsNegativeLimits(t *testing.T) {
	t.Parallel()

	_, err := merchantwebhookhttp.New(merchantwebhookhttp.Config{
		Policy:   devPolicy,
		Timeout:  -time.Second,
		Executor: testHTTPClient(t),
	})
	require.Error(t, err)
}
