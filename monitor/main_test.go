package main

import (
	"fmt"
	"testing"

	"github.com/nov1n/kubernetes-heat-scheduling/monitor/pkg/heapster"
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

func TestComputeNewJoules(t *testing.T) {
	metrics, err := heapster.ParseMetrics(metricsJSON)
	if err != nil {
		t.Errorf("error parsing metrics: %v", err)
	}

	new, err := computeJoulesFromMetrics(metrics)
	fmt.Printf("joule:%v", new)
	if err != nil {
		t.Errorf("error computing joules: %v", err)
	}
	if new < 0 {
		t.Errorf("computed negative joules: %v", new)
	}
}
