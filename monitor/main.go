package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"time"

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
	updateInterval          = 30 * time.Second
	jouleScaleFactor        = 0.000000001
	heapsterService         = "http://heapster.kube-system" // DNS name for heapster service
	metricsEndpointTemplate = "/api/v1/model/nodes/%s/metrics/cpu/usage"
)

func main() {
	// Create client
	client, err := getClient()
	if err != nil {
		fmt.Printf("Error getting client: %v", err)
		return
	}
	fmt.Println("Created client")

	// Update loop
	stopChan := make(chan struct{})
	ticker := time.NewTicker(updateInterval)
	go func() {
		fmt.Println("Starting")
		lastUpdate := time.Now()
		for {
			select {
			case <-ticker.C:
				fmt.Println("Updating...")
				update(client, lastUpdate)
				lastUpdate = time.Now()
				fmt.Println("Update complete")
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
			return nil, fmt.Errorf("Could not create kubernetes client: %v", err)
		}
	}
	return client, nil
}

func update(client *k8sClient.Client, lastUpdate time.Time) error {
	// List nodes
	nodeClient := client.Nodes()
	listAll := api.ListOptions{LabelSelector: labels.Everything(), FieldSelector: fields.Everything()}
	nodes, err := nodeClient.List(listAll)
	if err != nil {
		return fmt.Errorf("Could not list nodes: %v", err)
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
		if metrics.LatestTimestamp.Before(lastUpdate) {
			// No new readings
			fmt.Printf("Skipped computing joules, no new readings, skipping ...\n")
			continue
		}

		// Compute new joule value
		oldJoulesLabel := node.Labels[joulesLabelName]
		oldJoules, err := strconv.ParseFloat(oldJoulesLabel, 64)
		if err != nil {
			fmt.Printf("Error updating node '%v':%v, skipping...\n", name, err)
			continue
		}

		difJoules, err := computeJoules(metrics, jouleScaleFactor)
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
		fmt.Printf("Updated joules label for node %s from %s to %s\n", name, oldJoulesLabel, newJoulesLabel)
	}
	return nil
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
		return nil, fmt.Errorf("could not get metrics at endpoint %s: %v", endpoint, err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read response body: %v", err)
	}
	return body, nil
}

// parseMetrics parses the metrics JSON into metrics struct
func parseMetrics(metricsJSON []byte) (*metrics, error) {
	metrics := &metrics{}
	err := json.Unmarshal(metricsJSON, &metrics)
	if err != nil {
		return nil, fmt.Errorf("could not decode metrics response into json: %v", err)
	}
	return metrics, nil
}

// computeJoules uses the metrics to calculate the new joule value for a node
func computeJoules(metrics *metrics, jouleScaleFactor float64) (float64, error) {
	readings := metrics.Readings
	l := len(readings)
	fmt.Print(l)
	joules := (readings[l-1].Value - readings[l-2].Value) * jouleScaleFactor
	return joules, nil
}
