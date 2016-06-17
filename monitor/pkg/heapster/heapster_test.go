package heapster

import "testing"

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

func TestParseMetrics(t *testing.T) {
	_, err := ParseMetrics(metricsJSON)
	if err != nil {
		t.Errorf("error parsing metrics: %v", err)
	}
}
