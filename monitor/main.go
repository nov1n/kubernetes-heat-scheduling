package main

import (
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/nov1n/kubernetes-heat-scheduling/monitor/pkg/heapster"
	"github.com/nov1n/kubernetes-heat-scheduling/monitor/pkg/influx"

	k8sApi "k8s.io/kubernetes/pkg/api"
	k8sApiErr "k8s.io/kubernetes/pkg/api/errors"
	k8sRestCl "k8s.io/kubernetes/pkg/client/restclient"
	k8sClient "k8s.io/kubernetes/pkg/client/unversioned"
	k8sFields "k8s.io/kubernetes/pkg/fields"
	k8sLabels "k8s.io/kubernetes/pkg/labels"
)

const (
	joulesLabelName       = "joules"
	k8sHost               = "127.0.0.1"
	k8sPort               = "8080"
	updateInterval        = 10 * time.Second
	scaleFactor           = 0.0000000003
	retryOnStatusConflict = 3
	scaleFactorEnv        = "SCALE_FACTOR"
)

var (
	// records when a node is last updated
	mu         = sync.Mutex{} // guards lastUpdate
	lastUpdate = make(map[string]time.Time)
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
	influx, err := influx.NewClient()
	if err != nil {
		fmt.Printf("Error creating influx client: %v\n", err)
		return
	}
	fmt.Println("Created influx client")

	// Update loop
	func() {
		fmt.Println("Starting update loop")
		for {
			fmt.Println("Updating...")
			update(client, influx, lastUpdate)
			fmt.Println("Update finished")
			time.Sleep(updateInterval)
		}
	}()
}

// getClient returns a kubernetes client
func getClient() (*k8sClient.Client, error) {
	// If we are running in a pod, this is the easiest way
	client, err := k8sClient.NewInCluster()
	if err != nil {
		// We are not running in a pod so create a regular HTTP client
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

// update is called every upupdateInterval and updates the joules labels on all nodes
func update(client *k8sClient.Client, influx *influx.Client, lastUpdate map[string]time.Time) {
	// List nodes
	nodeClient := client.Nodes()
	listAll := k8sApi.ListOptions{LabelSelector: k8sLabels.Everything(), FieldSelector: k8sFields.Everything()}
	nodes, err := nodeClient.List(listAll)
	if err != nil {
		fmt.Printf("Could not list nodes, trying again in %v: %v\n", updateInterval, err)
		return
	}
	fmt.Printf("Listed %v nodes\n", len(nodes.Items))

	// Create channel for goroutine synchronization
	readyChan := make(chan bool, len(nodes.Items))

	// Update every node
	for _, node := range nodes.Items {
		go processNode(nodeClient, influx, node, readyChan)
	}

	// Wait for all updates to complete
	for i := 0; i < len(nodes.Items); i++ {
		<-readyChan
	}
}

// processNode updates an individual node if new readings are available from heapster
func processNode(nodeClient k8sClient.NodeInterface, influx *influx.Client, node k8sApi.Node, readyChan chan bool) {
	defer func() { readyChan <- true }() // Report on readyChan when done
	name := node.Name

	// Get metrics
	metrics, err := heapster.GetMetrics(name)
	if err != nil {
		fmt.Printf("Error updating node '%v':%v, skipping...\n", name, err)
		return
	}

	// Check if new readings came in
	err = hasNewReadings(name, metrics)
	if err != nil {
		fmt.Printf("Skipped computing joules for node `%s`: %v\n", name, err)
		return
	}

	// Assign new joule value as label
	newJoules, err := computeNewJoules(node, metrics)
	if err != nil {
		fmt.Printf("Could not compute joules for node `%s`: %v\n", name, err)
		return
	}
	newJoulesLabel := fmt.Sprintf("%.2f", newJoules)
	node.Labels[joulesLabelName] = newJoulesLabel

	// Send the modified node object on the apiserver
	err = updateNode(nodeClient, node, newJoulesLabel)
	if err != nil {
		fmt.Printf("Error updating node '%v':%v, skipping...\n", name, err)
		return
	}

	// Write updated joules to influx
	err = influx.Insert(name, newJoules)
	if err != nil {
		fmt.Printf("Error updating node '%v':%v, skipping...\n", name, err)
		return
	}

	fmt.Printf("Updated joules label for node %s", name)
}

// updateNode returns nil if successful and an error otherwise
func updateNode(nodeClient k8sClient.NodeInterface, node k8sApi.Node, newJoulesLabel string) error {
	name := node.Name
	for i := 0; ; i++ {
		// Try update
		_, updateErr := nodeClient.Update(&node)
		if updateErr == nil {
			// Success
			return nil
		}

		// Check if we have reached retry limit
		if i >= retryOnStatusConflict {
			return fmt.Errorf("tried to update status of node %v, but amount of retries (%d) exceeded", name, retryOnStatusConflict)
		}

		// Check if it is a status error 409
		statusErr, ok := updateErr.(*k8sApiErr.StatusError)
		if !ok {
			return fmt.Errorf("tried to update status of node %v in retry %d/%d, but got error: %v", name, i, retryOnStatusConflict, updateErr)
		}

		// Get updated version from apiserver
		newNode, getErr := nodeClient.Get(name)
		if getErr != nil {
			return fmt.Errorf("tried to update status of node %v in retry %d/%d, but got error: %v", name, i, retryOnStatusConflict, getErr)
		}

		// Use newer version from API server, chanage labels and try to update again
		node = *newNode
		node.Labels[joulesLabelName] = newJoulesLabel
		fmt.Printf("Tried to update status of node %v in retry %d/%d, but encountered status error (%v), retrying", name, i, retryOnStatusConflict, statusErr)
	}
}

// computeJoulesLabel computes the new joules label for a node based on its old label and metrics
func computeNewJoules(node k8sApi.Node, metrics *heapster.Metrics) (float64, error) {
	oldJoulesLabel := node.Labels[joulesLabelName]
	oldJoules, err := strconv.ParseFloat(oldJoulesLabel, 64)
	if err != nil {
		return 0, err
	}

	difJoules, err := computeJoulesFromMetrics(metrics)
	if err != nil {
		return 0, err
	}

	newJoules := oldJoules + difJoules
	return newJoules, nil
}

// computeJoules uses the metrics to calculate the new joule value for a node
func computeJoulesFromMetrics(metrics *heapster.Metrics) (float64, error) {
	readings := metrics.Readings
	l := len(readings)
	if l < 2 {
		return 0, fmt.Errorf("Readings length was %v, must be at least 2", l)
	}

	joules := (readings[l-1].Value - readings[l-2].Value) * scaleFactor
	return joules, nil
}

// hasNewReadings returns nil if for a given node, heapster has new readings since we last updated
// an error is returned otherwise
func hasNewReadings(name string, metrics *heapster.Metrics) error {
	mu.Lock()
	if lastUpd, ok := lastUpdate[name]; ok && lastUpd.Equal(metrics.LatestTimestamp) {
		// No new readings
		mu.Unlock()
		return fmt.Errorf("no new readings")
	}
	lastUpdate[name] = metrics.LatestTimestamp
	mu.Unlock()
	return nil
}
