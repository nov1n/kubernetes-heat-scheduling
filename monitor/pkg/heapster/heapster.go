package heapster

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

const (
	metricsEndpointTemplate = "/api/v1/model/nodes/%s/metrics/cpu/usage"
	heapsterService         = "http://heapster.kube-system" // DNS name for heapster service
)

// Metrics represents the heapster json response
type Metrics struct {
	Readings        []Metric  `json:"metrics"`
	LatestTimestamp time.Time `json:"latestTimestamp"`
}

// Metric is a a single metric as returned by heapster
type Metric struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// GetMetrics queries heapster and returns a metrics struct
func GetMetrics(name string) (*Metrics, error) {
	// Fill in endpoint template
	endpoint := fmt.Sprintf(metricsEndpointTemplate, name)

	// Make request to heapster
	metricsJSON, err := getMetricsJSON(endpoint)
	if err != nil {
		return nil, err
	}

	// Parse response
	metrics, err := ParseMetrics(metricsJSON)
	if err != nil {
		return nil, err
	}

	return metrics, nil
}

// getMetricsJSON retrieves the latest metrics from heapster and returns the JSON
func getMetricsJSON(endpoint string) ([]byte, error) {
	resp, err := http.Get(heapsterService + endpoint)
	if err != nil {
		return nil, fmt.Errorf("could not get metrics at endpoint %s: %v\n", endpoint, err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read response body: %v\n", err)
	}
	return body, nil
}

// ParseMetrics parses the metrics JSON into metrics struct
func ParseMetrics(metricsJSON []byte) (*Metrics, error) {
	metrics := &Metrics{}
	err := json.Unmarshal(metricsJSON, &metrics)
	if err != nil {
		return nil, fmt.Errorf("could not decode metrics response into json: %v\n", err)
	}
	return metrics, nil
}
