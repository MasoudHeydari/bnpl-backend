package standard_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	httpstandard "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/httpclient/standard"
	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	metricsnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics/noop"
	corehttpclient "github.com/parstis/bnpl-backend/pkg/core/httpclient"
	webhookpolicy "github.com/parstis/bnpl-backend/pkg/core/policy/webhook"
)

type fakeResolver struct {
	addrs []netip.Addr
}

func (r fakeResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return r.addrs, nil
}

func newStandardClient(t *testing.T, timeout time.Duration) *httpstandard.Client {
	t.Helper()

	client, err := httpstandard.New(
		httpstandard.Config{
			MaxInFlight:           4,
			MaxQueueSize:          4,
			QueueWaitTimeout:      time.Second,
			Timeout:               timeout,
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

func TestSecureProfile_IndependentTimeout(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		select {
		case <-release:

		case <-time.After(2 * time.Second):
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	client := newStandardClient(t, 5*time.Second)
	profile, err := client.NewSecureProfile(corehttpclient.SecureProfileConfig{
		Name:          "webhook",
		NetworkPolicy: webhookpolicy.Development(),
		Timeout:       100 * time.Millisecond,
		DialTimeout:   time.Second,
	})
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL, nil)
	require.NoError(t, err)

	started := time.Now()
	_, err = client.DoWithSecureProfile(req, profile)
	elapsed := time.Since(started)

	require.Error(t, err)
	require.Less(t, elapsed, time.Second)
}

func TestSecureProfile_BlocksPrivateDNS(t *testing.T) {
	t.Parallel()

	client := newStandardClient(t, time.Second)
	profile, err := client.NewSecureProfile(corehttpclient.SecureProfileConfig{
		Name:          "secure",
		NetworkPolicy: webhookpolicy.Strict(),
		Timeout:       time.Second,
		DialTimeout:   time.Second,
	})
	require.NoError(t, err)
	require.NoError(t, client.SetSecureProfileResolver(profile, fakeResolver{
		addrs: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
	}))

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"http://internal.example.com/",
		nil,
	)
	require.NoError(t, err)

	_, err = client.DoWithSecureProfile(req, profile)
	require.Error(t, err)
	require.ErrorIs(t, err, webhookpolicy.ErrBlockedAddress)
}

func TestSecureProfile_RejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	client := newStandardClient(t, time.Second)

	_, err := client.NewSecureProfile(corehttpclient.SecureProfileConfig{
		Name:          "bad",
		NetworkPolicy: webhookpolicy.Strict(),
		Timeout:       0,
		DialTimeout:   time.Second,
	})
	require.Error(t, err)

	_, err = client.NewSecureProfile(corehttpclient.SecureProfileConfig{
		Name:        "bad2",
		Timeout:     time.Second,
		DialTimeout: time.Second,
	})
	require.Error(t, err)
}

func TestSecureProfile_ReusesSameHandle(t *testing.T) {
	t.Parallel()

	client := newStandardClient(t, time.Second)
	cfg := corehttpclient.SecureProfileConfig{
		Name:          "reuse",
		NetworkPolicy: webhookpolicy.Development(),
		Timeout:       time.Second,
		DialTimeout:   time.Second,
	}

	first, err := client.NewSecureProfile(cfg)
	require.NoError(t, err)

	second, err := client.NewSecureProfile(cfg)
	require.NoError(t, err)
	require.Same(t, first, second)
}

// Reusing a profile name must not silently weaken its security policy or
// change timeouts. This matters when different integrations share one client.
func TestSecureProfile_RejectsChangedConfiguration(t *testing.T) {
	t.Parallel()

	client := newStandardClient(t, time.Second)
	cfg := corehttpclient.SecureProfileConfig{
		Name:          "merchant-webhook",
		NetworkPolicy: webhookpolicy.Strict(),
		Timeout:       10 * time.Second,
		DialTimeout:   2 * time.Second,
	}
	_, err := client.NewSecureProfile(cfg)
	require.NoError(t, err)

	changedTimeout := cfg
	changedTimeout.Timeout = 30 * time.Second
	_, err = client.NewSecureProfile(changedTimeout)
	require.ErrorContains(t, err, "different configuration")

	weakerPolicy := cfg
	weakerPolicy.NetworkPolicy = webhookpolicy.Development()
	_, err = client.NewSecureProfile(weakerPolicy)
	require.ErrorContains(t, err, "different configuration")
}

// SecureProfile is an opaque handle scoped to its creating executor.
func TestSecureProfile_RejectsAnotherClientProfile(t *testing.T) {
	t.Parallel()

	first := newStandardClient(t, time.Second)
	second := newStandardClient(t, time.Second)
	profile, err := first.NewSecureProfile(corehttpclient.SecureProfileConfig{
		Name: "owner", NetworkPolicy: webhookpolicy.Strict(),
		Timeout: time.Second, DialTimeout: time.Second,
	})
	require.NoError(t, err)

	require.ErrorContains(t, second.SetSecureProfileResolver(profile, fakeResolver{}), "another client")
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com", nil)
	require.NoError(t, err)
	_, err = second.DoWithSecureProfile(req, profile)
	require.ErrorContains(t, err, "another client")
}
