package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/influxdata/influxdb/client/v2"

	"k8s.io/kubernetes/pkg/api"
	k8sRestCl "k8s.io/kubernetes/pkg/client/restclient"
	k8sClient "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/fields"
	"k8s.io/kubernetes/pkg/labels"
)

const (
	joulesLabelName         = "joules"
	k8sHost                 = "127.0.0.1"
	k8sPort                 = "8080"
	updateInterval          = 5 * time.Second
	defaultScaleFactor      = "0.0000000001"
	scaleFactorEnv          = "SCALE_FACTOR"
	heapsterService         = "http://heapster-service.default" // DNS name for heapster service
	metricsEndpointTemplate = "/api/v1/model/nodes/%s/metrics/cpu/usage"
)

func main() {
	// Create client
	client, err := getClient()
	if err != nil {
		fmt.Printf("Error getting client: %v\n", err)
		return
	}
	fmt.Println("Created client")

	// Create Influx client
	influx, err := NewInfluxClient()
	if err != nil {
		fmt.Printf("Error creating influx client: %v\n", err)
		return
	}
	fmt.Println("Created influx client")

	// Set joules scale factor environment variable if not set
	factorExists := os.Getenv(scaleFactorEnv) != ""
	if !factorExists {
		os.Setenv(scaleFactorEnv, defaultScaleFactor)
	}

	// Update loop
	stopChan := make(chan struct{})
	ticker := time.NewTicker(updateInterval)
	go func() {
		fmt.Println("Starting")
		lastUpdate := make(map[string]time.Time)
		for {
			select {
			case <-ticker.C:
				fmt.Println("Updating...")
				update(client, influx, lastUpdate)
				fmt.Println("Update finished")
			case <-stopChan:
				ticker.Stop()
				return
			}
		}
	}()
	<-stopChan
}

func getClient() (*k8sClient.Client, error) {
	client, err := k8sClient.NewInCluster()
	if err != nil {
		clientConfig := k8sRestCl.Config{
			Host: "http://" + net.JoinHostPort(k8sHost, k8sPort),
		}
		client, err = k8sClient.New(&clientConfig)
		if err != nil {
			return nil, fmt.Errorf("Could not create kubernetes client: %v\n", err)
		}
	}
	return client, nil
}

func update(client *k8sClient.Client, influx *Client, lastUpdate map[string]time.Time) {
	// List nodes
	nodeClient := client.Nodes()
	listAll := api.ListOptions{LabelSelector: labels.Everything(), FieldSelector: fields.Everything()}
	nodes, err := nodeClient.List(listAll)
	if err != nil {
		fmt.Printf("Could not list nodes, trying again in %v: %v\n", updateInterval, err)
		return
	}
	fmt.Printf("Listed %v nodes\n", len(nodes.Items))

	// Update every node
	for _, node := range nodes.Items {
		name := node.Name

		// Get metrics
		metricsEndpoint := fmt.Sprintf(metricsEndpointTemplate, name)
		metricsJSON, err := getMetricsJSON(metricsEndpoint)
		if err != nil {
			fmt.Printf("Error updating node '%v':%v, skipping...\n", name, err)
			continue
		}

		// Parse metrics
		metrics, err := parseMetrics(metricsJSON)
		if err != nil {
			fmt.Printf("Error updating node '%v':%v, skipping...\n", name, err)
			continue
		}

		// Check if new readings came in
		if lastUpd, ok := lastUpdate[name]; ok && lastUpd.Equal(metrics.LatestTimestamp) {
			// No new readings
			fmt.Printf("Skipped computing joules for node `%s`, no new readings...\n", name)
			continue
		}
		lastUpdate[name] = metrics.LatestTimestamp

		// Compute new joule value
		oldJoulesLabel := node.Labels[joulesLabelName]
		oldJoules, err := strconv.ParseFloat(oldJoulesLabel, 64)
		if err != nil {
			fmt.Printf("Error updating node '%v':%v, skipping...\n", name, err)
			continue
		}

		scaleFactor := os.Getenv(scaleFactorEnv)
		fmt.Printf("Scale factor: '%s'\t", scaleFactor)

		difJoules, err := computeJoules(metrics, scaleFactor)
		if err != nil {
			fmt.Printf("Error updating node '%v':%v, skipping...\n", name, err)
			continue
		}

		newJoules := oldJoules + difJoules
		newJoulesLabel := fmt.Sprintf("%.2f", newJoules)

		// Update node
		node.Labels[joulesLabelName] = newJoulesLabel
		_, err = nodeClient.Update(&node)
		if err != nil {
			fmt.Printf("Error updating node '%v':%v, skipping...\n", name, err)
			continue
		}

		// Write updated joules to influx
		err = influx.Insert(name, newJoules)
		if err != nil {
			fmt.Printf("Error updating node '%v':%v, skipping...\n", name, err)
			continue
		}

		fmt.Printf("Updated joules label for node %s from %s to %s\n", name, oldJoulesLabel, newJoulesLabel)
	}
}

type metrics struct {
	Readings        []metric  `json:"metrics"`
	LatestTimestamp time.Time `json:"latestTimestamp"`
}

type metric struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
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

// parseMetrics parses the metrics JSON into metrics struct
func parseMetrics(metricsJSON []byte) (*metrics, error) {
	metrics := &metrics{}
	err := json.Unmarshal(metricsJSON, &metrics)
	if err != nil {
		return nil, fmt.Errorf("could not decode metrics response into json: %v\n", err)
	}
	return metrics, nil
}

// computeJoules uses the metrics to calculate the new joule value for a node
func computeJoules(metrics *metrics, jouleScaleFactor string) (float64, error) {
	readings := metrics.Readings
	l := len(readings)

	// Parse factor from string
	parsedFactor, err := strconv.ParseFloat(jouleScaleFactor, 64)
	if err != nil {
		return 0, err
	}

	joules := (readings[l-1].Value - readings[l-2].Value) * parsedFactor
	return joules, nil
}

const (
	service   = "http://nce-pm-influxdb.default"
	db        = "k8s"
	port      = "8086"
	username  = "root"
	password  = "root"
	hostLabel = "hostname"
)

// Client is able to insert Points into InfluxDB
type Client struct {
	HTTP client.Client
}

// NewInfluxClient returns a new Influx object
func NewInfluxClient() (*Client, error) {
	// Make client
	c, err := client.NewHTTPClient(client.HTTPConfig{
		Addr:     fmt.Sprintf("%s:%s", service, port),
		Password: password,
	})

	if err != nil {
		return nil, err
	}

	// Create database if it does not yet exist
	query := client.NewQuery(
		"CREATE DATABASE IF NOT EXISTS joules",
		db,
		"s",
	)
	_, err = c.Query(query)
	if err != nil {
		return nil, err
	}

	// Return Influx instance
	return &Client{
		HTTP: c,
	}, nil
}

// Insert creates and inserts a Point for a given podName and joule value
func (c *Client) Insert(nodeName string, joulesFloat float64) error {
	// Create a point
	tags := map[string]string{
		hostLabel: nodeName,
	}
	fields := map[string]interface{}{
		"total": joulesFloat,
	}
	pt, err := client.NewPoint("joules", tags, fields, time.Now())
	if err != nil {
		return err
	}

	// Create a new point batch
	bp, err := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  db,
		Precision: "s",
	})
	bp.AddPoint(pt)

	// Write point to influx
	err = c.HTTP.Write(bp)
	if err != nil {
		return err
	}

	return nil
}
