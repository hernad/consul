package hcp

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/armon/go-metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul/agent/hcp/client"
)

const (
	testRefreshInterval      = 100 * time.Millisecond
	testSinkServiceName      = "test.telemetry_config_provider"
	testRaceWriteSampleCount = 100
	testRaceReadSampleCount  = 5000
)

var (
	// Test constants to verify inmem sink metrics.
	testMetricKeyFailure = testSinkServiceName + "." + strings.Join(internalMetricRefreshFailure, ".")
	testMetricKeySuccess = testSinkServiceName + "." + strings.Join(internalMetricRefreshSuccess, ".")
)

type testConfig struct {
	filters         string
	endpoint        string
	labels          map[string]string
	refreshInterval time.Duration
	disabled        bool
}

func TestNewTelemetryConfigProvider(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		mock            func(*client.MockClient)
		expectedTestCfg *testConfig
	}{
		"initWithDefaultConfig": {
			mock: func(mockClient *client.MockClient) {
				mockClient.EXPECT().FetchTelemetryConfig(mock.Anything).Return(nil, errors.New("failed to fetch config"))
			},
			expectedTestCfg: &testConfig{
				refreshInterval: defaultTelemetryConfigRefreshInterval,
				filters:         client.DefaultMetricFilters.String(),
				labels:          map[string]string{},
				disabled:        true,
				endpoint:        "",
			},
		},
		"initWithFirstUpdate": {
			mock: func(mockClient *client.MockClient) {
				cfg, err := testTelemetryCfg(&testConfig{
					refreshInterval: 2 * time.Second,
					filters:         "consul",
					labels:          map[string]string{"test_label": "123"},
					endpoint:        "https://test.com/v1/metrics",
				})
				require.NoError(t, err)
				mockClient.EXPECT().FetchTelemetryConfig(mock.Anything).Return(cfg, nil)
			},
			expectedTestCfg: &testConfig{
				refreshInterval: 2 * time.Second,
				filters:         "consul",
				labels:          map[string]string{"test_label": "123"},
				endpoint:        "https://test.com/v1/metrics",
			},
		},
	} {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			mc := client.NewMockClient(t)
			tc.mock(mc)

			provider := NewHCPProvider(ctx, mc)

			expectedCfg, err := testDynamicCfg(tc.expectedTestCfg)
			require.NoError(t, err)

			require.Equal(t, expectedCfg, provider.cfg)
		})
	}
}

func TestTelemetryConfigProviderGetUpdate(t *testing.T) {
	for name, tc := range map[string]struct {
		mockExpect       func(*client.MockClient)
		metricKey        string
		optsInputs       *testConfig
		expected         *testConfig
		expectedInterval time.Duration
	}{
		"noChanges": {
			optsInputs: &testConfig{
				endpoint: "http://test.com/v1/metrics",
				filters:  "test",
				labels: map[string]string{
					"test_label": "123",
				},
				refreshInterval: testRefreshInterval,
			},
			mockExpect: func(m *client.MockClient) {
				mockCfg, _ := testTelemetryCfg(&testConfig{
					endpoint: "http://test.com/v1/metrics",
					filters:  "test",
					labels: map[string]string{
						"test_label": "123",
					},
					refreshInterval: testRefreshInterval,
				})
				m.EXPECT().FetchTelemetryConfig(mock.Anything).Return(mockCfg, nil)
			},
			expected: &testConfig{
				endpoint: "http://test.com/v1/metrics",
				labels: map[string]string{
					"test_label": "123",
				},
				filters:         "test",
				refreshInterval: testRefreshInterval,
			},
			metricKey:        testMetricKeySuccess,
			expectedInterval: testRefreshInterval,
		},
		"newConfig": {
			optsInputs: &testConfig{
				endpoint: "http://test.com/v1/metrics",
				filters:  "test",
				labels: map[string]string{
					"test_label": "123",
				},
				refreshInterval: 2 * time.Second,
			},
			mockExpect: func(m *client.MockClient) {
				mockCfg, _ := testTelemetryCfg(&testConfig{
					endpoint: "http://newendpoint/v1/metrics",
					filters:  "consul",
					labels: map[string]string{
						"new_label": "1234",
					},
					refreshInterval: 2 * time.Second,
				})
				m.EXPECT().FetchTelemetryConfig(mock.Anything).Return(mockCfg, nil)
			},
			expected: &testConfig{
				endpoint: "http://newendpoint/v1/metrics",
				filters:  "consul",
				labels: map[string]string{
					"new_label": "1234",
				},
				refreshInterval: 2 * time.Second,
			},
			expectedInterval: 2 * time.Second,
			metricKey:        testMetricKeySuccess,
		},
		"newConfigMetricsDisabled": {
			optsInputs: &testConfig{
				endpoint: "http://test.com/v1/metrics",
				filters:  "test",
				labels: map[string]string{
					"test_label": "123",
				},
				refreshInterval: 2 * time.Second,
			},
			mockExpect: func(m *client.MockClient) {
				mockCfg, _ := testTelemetryCfg(&testConfig{
					endpoint: "",
					filters:  "consul",
					labels: map[string]string{
						"new_label": "1234",
					},
					refreshInterval: 2 * time.Second,
				})
				m.EXPECT().FetchTelemetryConfig(mock.Anything).Return(mockCfg, nil)
			},
			expected: &testConfig{
				endpoint: "",
				filters:  "consul",
				labels: map[string]string{
					"new_label": "1234",
				},
				refreshInterval: 2 * time.Second,
				disabled:        true,
			},
			metricKey:        testMetricKeySuccess,
			expectedInterval: 2 * time.Second,
		},
		"sameConfigInvalidRefreshInterval": {
			optsInputs: &testConfig{
				endpoint: "http://test.com/v1/metrics",
				filters:  "test",
				labels: map[string]string{
					"test_label": "123",
				},
				refreshInterval: testRefreshInterval,
			},
			mockExpect: func(m *client.MockClient) {
				mockCfg, _ := testTelemetryCfg(&testConfig{
					refreshInterval: 0 * time.Second,
				})
				m.EXPECT().FetchTelemetryConfig(mock.Anything).Return(mockCfg, nil)
			},
			expected: &testConfig{
				endpoint: "http://test.com/v1/metrics",
				labels: map[string]string{
					"test_label": "123",
				},
				filters:         "test",
				refreshInterval: testRefreshInterval,
			},
			metricKey:        testMetricKeyFailure,
			expectedInterval: 0,
		},
		"sameConfigHCPClientFailure": {
			optsInputs: &testConfig{
				endpoint: "http://test.com/v1/metrics",
				filters:  "test",
				labels: map[string]string{
					"test_label": "123",
				},
				refreshInterval: testRefreshInterval,
			},
			mockExpect: func(m *client.MockClient) {
				m.EXPECT().FetchTelemetryConfig(mock.Anything).Return(nil, fmt.Errorf("failure"))
			},
			expected: &testConfig{
				endpoint: "http://test.com/v1/metrics",
				filters:  "test",
				labels: map[string]string{
					"test_label": "123",
				},
				refreshInterval: testRefreshInterval,
			},
			metricKey:        testMetricKeyFailure,
			expectedInterval: 0,
		},
	} {
		tc := tc
		t.Run(name, func(t *testing.T) {
			sink := initGlobalSink()
			mockClient := client.NewMockClient(t)
			tc.mockExpect(mockClient)

			dynamicCfg, err := testDynamicCfg(tc.optsInputs)
			require.NoError(t, err)

			provider := &hcpProviderImpl{
				hcpClient: mockClient,
				cfg:       dynamicCfg,
			}

			newInterval := provider.updateConfig(context.Background())
			require.Equal(t, tc.expectedInterval, newInterval)

			// Verify endpoint provider returns correct config values.
			expectedCfg, err := testDynamicCfg(tc.expected)
			require.NoError(t, err)

			require.Equal(t, expectedCfg.endpoint, provider.GetEndpoint())
			require.Equal(t, expectedCfg.filters, provider.GetFilters())
			require.Equal(t, expectedCfg.labels, provider.GetLabels())
			require.Equal(t, expectedCfg.disabled, provider.IsDisabled())

			// Verify count for transform success metric.
			interval := sink.Data()[0]
			require.NotNil(t, interval, 1)
			sv := interval.Counters[tc.metricKey]
			assert.NotNil(t, sv.AggregateSample)
			require.Equal(t, sv.AggregateSample.Count, 1)
		})
	}
}

// mockRaceClient is a mock HCP client that fetches TelemetryConfig.
// The mock TelemetryConfig returned can be manually updated at any time.
// It manages concurrent read/write access to config with a sync.RWMutex.
type mockRaceClient struct {
	client.Client
	cfg *client.TelemetryConfig
	rw  sync.RWMutex
}

// updateCfg acquires a write lock and updates client config to a new value givent a count.
func (m *mockRaceClient) updateCfg(count int) (*client.TelemetryConfig, error) {
	m.rw.Lock()
	defer m.rw.Unlock()

	labels := map[string]string{fmt.Sprintf("label_%d", count): fmt.Sprintf("value_%d", count)}

	filters, err := regexp.Compile(fmt.Sprintf("consul_filter_%d", count))
	if err != nil {
		return nil, err
	}

	endpoint, err := url.Parse(fmt.Sprintf("http://consul-endpoint-%d.com", count))
	if err != nil {
		return nil, err
	}

	cfg := &client.TelemetryConfig{
		MetricsConfig: &client.MetricsConfig{
			Filters:  filters,
			Endpoint: endpoint,
			Labels:   labels,
		},
		RefreshConfig: &client.RefreshConfig{
			RefreshInterval: testRefreshInterval,
		},
	}
	m.cfg = cfg

	return cfg, nil
}

// FetchTelemetryConfig returns the current config held by the mockRaceClient.
func (m *mockRaceClient) FetchTelemetryConfig(ctx context.Context) (*client.TelemetryConfig, error) {
	m.rw.RLock()
	defer m.rw.RUnlock()

	return m.cfg, nil
}

func TestTelemetryConfigProvider_Race(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	initCfg, err := testTelemetryCfg(&testConfig{
		endpoint:        "test.com",
		filters:         "test",
		labels:          map[string]string{"test_label": "test_value"},
		refreshInterval: testRefreshInterval,
	})
	require.NoError(t, err)

	m := &mockRaceClient{
		cfg: initCfg,
	}

	// Start the provider goroutine, which fetches client TelemetryConfig every RefreshInterval.
	provider := NewHCPProvider(ctx, m)

	for count := 0; count < testRaceWriteSampleCount; count++ {
		// Force a TelemetryConfig value change in the mockRaceClient.
		newCfg, err := m.updateCfg(count)
		require.NoError(t, err)
		// Force provider to obtain new client TelemetryConfig immediately.
		// This call is necessary to guarantee TelemetryConfig changes to assert on expected values below.
		provider.updateConfig(context.Background())

		// Start goroutines to access label configuration.
		wg := &sync.WaitGroup{}
		kickOff(wg, testRaceReadSampleCount, provider, func(provider *hcpProviderImpl) {
			require.Equal(t, provider.GetLabels(), newCfg.MetricsConfig.Labels)
		})

		// Start goroutines to access endpoint configuration.
		kickOff(wg, testRaceReadSampleCount, provider, func(provider *hcpProviderImpl) {
			require.Equal(t, provider.GetFilters(), newCfg.MetricsConfig.Filters)
		})

		// Start goroutines to access filter configuration.
		kickOff(wg, testRaceReadSampleCount, provider, func(provider *hcpProviderImpl) {
			require.Equal(t, provider.GetEndpoint(), newCfg.MetricsConfig.Endpoint)
		})

		wg.Wait()
	}
}

func kickOff(wg *sync.WaitGroup, count int, provider *hcpProviderImpl, check func(cfgProvider *hcpProviderImpl)) {
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			check(provider)
		}()
	}
}

// initGlobalSink is a helper function to initialize a Go metrics inmemsink.
func initGlobalSink() *metrics.InmemSink {
	cfg := metrics.DefaultConfig(testSinkServiceName)
	cfg.EnableHostname = false

	sink := metrics.NewInmemSink(10*time.Second, 10*time.Second)
	metrics.NewGlobal(cfg, sink)

	return sink
}

// testDynamicCfg converts testConfig inputs to a dynamicConfig to be used in tests.
func testDynamicCfg(testCfg *testConfig) (*dynamicConfig, error) {
	filters, err := regexp.Compile(testCfg.filters)
	if err != nil {
		return nil, err
	}

	var endpoint *url.URL
	if testCfg.endpoint != "" {
		u, err := url.Parse(testCfg.endpoint)
		if err != nil {
			return nil, err
		}
		endpoint = u
	}
	return &dynamicConfig{
		endpoint:        endpoint,
		filters:         filters,
		labels:          testCfg.labels,
		refreshInterval: testCfg.refreshInterval,
		disabled:        testCfg.disabled,
	}, nil
}

// testTelemetryCfg converts testConfig inputs to a TelemetryConfig to be used in tests.
func testTelemetryCfg(testCfg *testConfig) (*client.TelemetryConfig, error) {
	filters, err := regexp.Compile(testCfg.filters)
	if err != nil {
		return nil, err
	}

	var endpoint *url.URL
	if testCfg.endpoint != "" {
		u, err := url.Parse(testCfg.endpoint)
		if err != nil {
			return nil, err
		}
		endpoint = u
	}

	return &client.TelemetryConfig{
		MetricsConfig: &client.MetricsConfig{
			Endpoint: endpoint,
			Filters:  filters,
			Labels:   testCfg.labels,
		},
		RefreshConfig: &client.RefreshConfig{
			RefreshInterval: testCfg.refreshInterval,
		},
	}, nil
}
