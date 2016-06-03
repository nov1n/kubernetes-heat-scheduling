package main

import (
	"fmt"
	"strconv"
	"testing"
)

var metricsJSON = []byte(`
	{
		"metrics": [
			{
				"timestamp": "2016-06-02T14:56:00Z",
				"value": 1675550639110
			},
			{
				"timestamp": "2016-06-02T14:57:00Z",
				"value": 1677252246036
			},
			{
				"timestamp": "2016-06-02T14:58:00Z",
				"value": 1678697169283
			}
		],
		"latestTimestamp": "2016-06-03T14:58:00Z"
	}`)

func TestParseMetricsJSON(t *testing.T) {
	_, err := parseMetrics(metricsJSON)
	if err != nil {
		t.Errorf("error parsing metrics: %v", err)
	}
}

func TestComputeJoules(t *testing.T) {
	metrics, err := parseMetrics(metricsJSON)
	if err != nil {
		t.Errorf("error parsing metrics: %v", err)
	}

	old := "5"
	new, err := computeJoules(old, metrics, jouleScaleFactor)
	fmt.Printf("joule:%v", new)
	if err != nil {
		t.Errorf("error computing joules: %v", err)
	}
	newVal, err := strconv.ParseFloat(new, 64)
	if err != nil {
		t.Errorf("joules value was not a float: %v", err)
	}
	if newVal < 0 {
		t.Errorf("computed negative joules: %v", new)
	}
}
