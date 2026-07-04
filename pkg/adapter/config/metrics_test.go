package config

import (
	"testing"

	adaptermetrics "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics"
	"github.com/stretchr/testify/require"
)

func TestMetricsValidate_ValidPrometheus(t *testing.T) {
	t.Parallel()

	m := Metrics{
		Provider: adaptermetrics.PrometheusProvider,
		Prometheus: PrometheusMetrics{
			Enabled: true,
			Host:    "0.0.0.0",
			Port:    9090,
			Path:    "/metrics",
		},
	}
	require.NoError(t, m.Validate())
	require.True(t, m.ServerEnabled())
	require.Equal(t, "0.0.0.0:9090", m.Prometheus.getListeningAddress())
}

func TestMetricsValidate_DisabledSkipsListenValidation(t *testing.T) {
	t.Parallel()

	m := Metrics{
		Provider: adaptermetrics.PrometheusProvider,
		Prometheus: PrometheusMetrics{
			Enabled: false,
			Port:    0,
			Path:    "/metrics",
		},
	}
	require.NoError(t, m.Validate())
	require.False(t, m.ServerEnabled())
}

func TestMetricsValidate_Noop(t *testing.T) {
	t.Parallel()

	m := Metrics{Provider: adaptermetrics.NoopProvider}
	require.NoError(t, m.Validate())
	require.False(t, m.ServerEnabled())
}

func TestMetricsValidate_InvalidPort(t *testing.T) {
	t.Parallel()

	m := Metrics{
		Provider: adaptermetrics.PrometheusProvider,
		Prometheus: PrometheusMetrics{
			Enabled: true,
			Host:    "127.0.0.1",
			Port:    70000,
			Path:    "/metrics",
		},
	}
	require.Error(t, m.Validate())
}

func TestMetricsValidate_InvalidPath(t *testing.T) {
	t.Parallel()

	m := Metrics{
		Provider: adaptermetrics.PrometheusProvider,
		Prometheus: PrometheusMetrics{
			Enabled: true,
			Host:    "127.0.0.1",
			Port:    9090,
			Path:    "metrics",
		},
	}
	require.Error(t, m.Validate())
}

func TestMetricsValidate_InvalidHost(t *testing.T) {
	t.Parallel()

	m := Metrics{
		Provider: adaptermetrics.PrometheusProvider,
		Prometheus: PrometheusMetrics{
			Enabled: true,
			Host:    "0.0.0.0:9090",
			Port:    9090,
			Path:    "/metrics",
		},
	}
	require.Error(t, m.Validate())
}

func TestMetrics_NewInfrastructureValidates(t *testing.T) {
	t.Parallel()

	_, _, err := Metrics{
		Provider: adaptermetrics.PrometheusProvider,
		Prometheus: PrometheusMetrics{
			Enabled: true,
			Port:    -1,
			Path:    "/metrics",
		},
	}.newMetricsInfrastructure()
	require.Error(t, err)
}
