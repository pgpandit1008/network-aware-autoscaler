package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"k8s.io/klog/v2"
)

// PrometheusClient queries Prometheus for scaling metrics
type PrometheusClient struct {
	baseURL    string
	httpClient *http.Client
}

// MetricsSnapshot holds a point-in-time snapshot of all scaling metrics
type MetricsSnapshot struct {
	CPUPercent   float64
	MemoryPercent float64
	P95LatencyMs  float64
	Timestamp     time.Time
}

type prometheusResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []interface{}     `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

// NewPrometheusClient creates a new Prometheus metrics client
func NewPrometheusClient(baseURL string) *PrometheusClient {
	return &PrometheusClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetMetrics queries Prometheus and returns a MetricsSnapshot for the given deployment
func (p *PrometheusClient) GetMetrics(ctx context.Context, namespace, deployment string) (*MetricsSnapshot, error) {
	snapshot := &MetricsSnapshot{Timestamp: time.Now()}

	cpu, err := p.queryScalar(ctx, fmt.Sprintf(
		`avg(rate(container_cpu_usage_seconds_total{namespace="%s",pod=~"%s-.*"}[2m])) * 100`,
		namespace, deployment,
	))
	if err != nil {
		klog.Warningf("Failed to query CPU metrics for %s/%s: %v", namespace, deployment, err)
	} else {
		snapshot.CPUPercent = cpu
	}

	mem, err := p.queryScalar(ctx, fmt.Sprintf(
		`avg(container_memory_working_set_bytes{namespace="%s",pod=~"%s-.*"}) / avg(container_spec_memory_limit_bytes{namespace="%s",pod=~"%s-.*"}) * 100`,
		namespace, deployment, namespace, deployment,
	))
	if err != nil {
		klog.Warningf("Failed to query memory metrics for %s/%s: %v", namespace, deployment, err)
	} else {
		snapshot.MemoryPercent = mem
	}

	p95, err := p.queryScalar(ctx, fmt.Sprintf(
		`histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket{namespace="%s",deployment="%s"}[5m])) by (le)) * 1000`,
		namespace, deployment,
	))
	if err != nil {
		klog.Warningf("Failed to query p95 latency for %s/%s: %v", namespace, deployment, err)
	} else {
		snapshot.P95LatencyMs = p95
	}

	klog.V(4).Infof("Metrics for %s/%s: CPU=%.1f%% MEM=%.1f%% P95=%.1fms",
		namespace, deployment, snapshot.CPUPercent, snapshot.MemoryPercent, snapshot.P95LatencyMs)

	return snapshot, nil
}

// queryScalar executes a PromQL query and returns a single float64 value
func (p *PrometheusClient) queryScalar(ctx context.Context, query string) (float64, error) {
	url := fmt.Sprintf("%s/api/v1/query?query=%s", p.baseURL, query)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to query Prometheus: %w", err)
	}
	defer resp.Body.Close()

	var result prometheusResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("failed to decode response: %w", err)
	}

	if result.Status != "success" || len(result.Data.Result) == 0 {
		return 0, fmt.Errorf("no data returned for query")
	}

	valueStr, ok := result.Data.Result[0].Value[1].(string)
	if !ok {
		return 0, fmt.Errorf("unexpected value type in Prometheus response")
	}

	return strconv.ParseFloat(valueStr, 64)
}
