package merchant

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/parstis/bnpl-backend/pkg/core/circuitbreaker"
	coremerchant "github.com/parstis/bnpl-backend/pkg/core/external/merchant"
	"github.com/parstis/bnpl-backend/pkg/core/httpclient"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// Client uses a fixed internal HTTPS origin and a service token supplied at startup.
// The credential mechanism is provisional until the hosted Dev identity is agreed.
type Client struct {
	executor httpclient.SecureProfileExecutor
	profile  httpclient.SecureProfile
	base     string
	token    model.Secret
	breaker  circuitbreaker.CircuitBreaker
}

var _ coremerchant.Client = (*Client)(nil)

type internalOrigin struct{ host string }

func (p internalOrigin) CheckHost(host string) error {
	const op = "merchant.internalOrigin.CheckHost"

	if !strings.EqualFold(host, p.host) {
		return fmt.Errorf("%s: unexpected Merchant host", op)
	}

	return nil
}

func (p internalOrigin) CheckAddr(addr netip.Addr) error {
	const op = "merchant.internalOrigin.CheckAddr"

	if !addr.IsValid() ||
		addr.IsUnspecified() ||
		addr.IsMulticast() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLoopback() {
		return fmt.Errorf("%s: invalid internal Merchant address", op)
	}

	// Private addresses are intentional for a configured internal service origin.
	return nil
}

// New builds a reusable redirect-free HTTP profile without contacting Merchant.
func New(client httpclient.Client, breaker circuitbreaker.CircuitBreaker, baseURL string, token model.Secret) (*Client, error) {
	const op = "merchant.New"

	executor, ok := client.(httpclient.SecureProfileExecutor)
	if !ok || executor == nil {
		return nil, fmt.Errorf("%s: secure shared HTTP executor is required", op)
	}

	if breaker == nil {
		return nil, fmt.Errorf("%s: circuit breaker is required", op)
	}

	u, err := url.Parse(baseURL)
	if err != nil ||
		u.Scheme != "https" ||
		u.Hostname() == "" ||
		u.User != nil ||
		u.RawQuery != "" ||
		u.Fragment != "" ||
		(u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("%s: internal HTTPS origin is required", op)
	}

	if token.Reveal() == "" || strings.ContainsFunc(token.Reveal(), func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r) || r > unicode.MaxASCII
	}) {
		return nil, fmt.Errorf("%s: valid service bearer token is required", op)
	}

	profile, err := executor.NewSecureProfile(httpclient.SecureProfileConfig{
		Name:          "merchant",
		NetworkPolicy: internalOrigin{host: u.Hostname()},
		Timeout:       10 * time.Second,
		DialTimeout:   3 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: create secure profile: %w", op, err)
	}

	return &Client{
		executor: executor,
		profile:  profile,
		base:     strings.TrimRight(u.String(), "/"),
		token:    token,
		breaker:  breaker,
	}, nil
}

// GetParticipation reads the authoritative participation reference.
func (c *Client) GetParticipation(ctx context.Context, id model.MerchantParticipationRequestID) (coremerchant.Reference, error) {
	var result struct {
		Data referenceDTO `json:"data"`
	}
	err := c.do(
		ctx,
		http.MethodGet,
		id.String(),
		nil,
		&result,
	)
	if err != nil {
		return coremerchant.Reference{}, err
	}

	d := result.Data
	var declaration *model.MerchantSelfDeclaration
	if d.SelfDeclaration != nil {
		declaration = &model.MerchantSelfDeclaration{
			MaxAmountMinor:    d.SelfDeclaration.MaxAmountMinor,
			MaxAmountCurrency: d.SelfDeclaration.MaxAmountCurrency,
			MaxTenureMonths:   d.SelfDeclaration.MaxTenureMonths,
		}
	}

	return coremerchant.Reference{
		MerchantID:            d.MerchantID,
		RequestID:             d.RequestID,
		BusinessLine:          d.BusinessLine,
		Environment:           d.Environment,
		TradeName:             d.TradeName,
		BaseApplicationStatus: d.BaseApplicationStatus,
		SelfDeclaration:       declaration,
	}, nil
}

// ReportReady sends the immutable readiness report and returns the current projection.
func (c *Client) ReportReady(ctx context.Context, p coremerchant.Progress) (coremerchant.Snapshot, error) {
	const op = "merchant.Client.ReportReady"

	body, err := json.Marshal(progressDTO{
		EventID:          p.EventID,
		OwnerVersion:     p.OwnerVersion,
		Status:           "READY",
		AccountReference: p.AccountReference,
		Issues:           []issueDTO{},
	})
	if err != nil {
		return coremerchant.Snapshot{}, fmt.Errorf("%s: encode progress: %w", op, err)
	}

	var result struct {
		Data snapshotDTO `json:"data"`
	}
	err = c.do(
		ctx,
		http.MethodPut,
		p.RequestID.String()+"/progress",
		body,
		&result,
	)
	if err != nil {
		return coremerchant.Snapshot{}, err
	}

	d := result.Data
	snapshot := coremerchant.Snapshot{
		RequestID:    d.RequestID,
		BusinessLine: d.Code,
		Environment:  d.Environment,
		Status:       d.Status,
		OwnerVersion: d.OwnerVersion,
	}
	if d.AccountReference != nil {
		snapshot.AccountReference = *d.AccountReference
	}

	return snapshot, nil
}

func (c *Client) do(ctx context.Context, method, path string, body []byte, output any) error {
	const op = "merchant.Client.do"

	// A reachable Merchant returning a conclusive request/authorization error
	// is not a dependency outage. Preserve that error without tripping the circuit.
	var conclusiveErr error
	err := c.breaker.Execute(ctx, "merchant", func(ctx context.Context) error {
		callErr := c.doRequest(
			ctx,
			method,
			path,
			body,
			output,
		)
		if errors.Is(callErr, coremerchant.ErrBlocked) || errors.Is(callErr, coremerchant.ErrReferenceNotFound) {
			conclusiveErr = callErr
			return nil
		}

		return callErr
	})
	if err != nil {
		return fmt.Errorf("%s: Merchant call: %w", op, err)
	}

	return conclusiveErr
}

func (c *Client) doRequest(ctx context.Context, method, path string, body []byte, output any) error {
	const op = "merchant.Client.doRequest"

	req, err := http.NewRequestWithContext(ctx, method, c.base+"/internal/v1/business-lines/bnpl/requests/"+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%s: construct request: %w", op, err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token.Reveal())
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	response, err := c.executor.DoWithSecureProfile(req, c.profile)
	if err != nil {
		if response != nil && response.Body != nil {
			response.Body.Close()
		}

		return fmt.Errorf("%s: Merchant transport failure: %w", op, err)
	}

	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
		// Only the agreed success response can complete delivery.

	case http.StatusNotFound:
		if method == http.MethodGet {
			return coremerchant.ErrReferenceNotFound
		}

		return fmt.Errorf("%s: Merchant response (%d): %w", op, response.StatusCode, coremerchant.ErrBlocked)

	case http.StatusRequestTimeout, http.StatusTooManyRequests:
		return fmt.Errorf("%s: temporary Merchant response (%d)", op, response.StatusCode)

	default:
		if response.StatusCode >= 500 {
			return fmt.Errorf("%s: temporary Merchant response (%d)", op, response.StatusCode)
		}

		return fmt.Errorf("%s: Merchant response (%d): %w", op, response.StatusCode, coremerchant.ErrBlocked)
	}

	const maxBody = 1 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if err != nil {
		return fmt.Errorf("%s: read Merchant response: %w", op, err)
	}

	if len(data) > maxBody || json.Unmarshal(data, output) != nil {
		return fmt.Errorf("%s: invalid Merchant response: %w", op, coremerchant.ErrBlocked)
	}

	return nil
}
