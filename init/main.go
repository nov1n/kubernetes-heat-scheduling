package main

import (
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/http"
	"strconv"

	k8sApi "k8s.io/kubernetes/pkg/api"
	k8sApiErr "k8s.io/kubernetes/pkg/api/errors"
	k8sRestCl "k8s.io/kubernetes/pkg/client/restclient"
	k8sClient "k8s.io/kubernetes/pkg/client/unversioned"
)

const (
	k8sHost               = "127.0.0.1"
	k8sPort               = "8080"
	retryOnStatusConflict = 3        // # retries before giving up
	port                  = "8090"   // webserver port
	joulesLabelName       = "joules" // name of Kubernetes label assigned to a node
)

var client *k8sClient.Client
var lastLabels = make(map[string]string)

func main() {
	// Create nodeclient
	var err error
	client, err = getClient()
	if err != nil {
		fmt.Printf("Error getting client: %v", err)
		return
	}

	// Register handlers
	http.HandleFunc("/setup", setup)
	http.HandleFunc("/reset", reset)

	// Start webserver
	fmt.Printf("Listening on localhost%v", port)
	http.ListenAndServe(":"+port, nil)
}

// Setup computes initial joule value for each node from a normal distribution.
// The values are set as labels with key 'joules'.
// Example usage -- GET :8000/setup?mean=35&sigma=std
func setup(w http.ResponseWriter, r *http.Request) {
	mean := 50.0
	std := 25.0

	// Overwrite defaults
	params := r.URL.Query()
	parseFloatInto(&mean, params.Get("mean"))
	parseFloatInto(&std, params.Get("std"))
	fmt.Printf("mean='%v', std='%v'", mean, std)

	// List all nodes
	listAll := k8sApi.ListOptions{}
	nodes, err := client.Nodes().List(listAll)
	if err != nil {
		fmt.Printf("Could not list nodes: %v\n", err)
		return
	}

	// Make new map for last labels, these are used for /reset
	lastLabels = make(map[string]string)

	// Update each node
	for _, node := range nodes.Items {
		go processNode(node, mean, std)
	}
}

// processNode processes a single node, assigning a temprature label with a
// random value from a normal distribution with parameters mean and std
func processNode(node k8sApi.Node, mean, std float64) {
	// Log when done
	defer func() {
		fmt.Printf("Updated node %s\n", node.Name)
	}()

	// Compute joule value
	joules := normFloat(std, mean)
	joulesString := fmt.Sprintf("%.2f", joules) // Truncate at two decimals

	// Update the node
	err := updateNode(node, joulesString)
	if err != nil {
		fmt.Printf("Tried to update node %v but failed: %v", node.Name, err)
	}

	// Update lastlabels to allow resets
	lastLabels[node.Name] = joulesString
}

// updateNode updates a single node retrying retryOnStatusConflict times before giving up
func updateNode(node k8sApi.Node, joulesString string) error {
	// Retry on status conflict
	for i := 0; ; i++ {
		node.Labels[joulesLabelName] = joulesString
		// Update node
		_, updateErr := client.Nodes().Update(&node)
		if updateErr == nil {
			break
		}
		if i >= retryOnStatusConflict {
			return fmt.Errorf("tried to update status of node %v, but amount of retries (%d) exceeded\n", node.Name, retryOnStatusConflict)
		}
		statusErr, ok := updateErr.(*k8sApiErr.StatusError)
		if !ok {
			return fmt.Errorf("Tried to update status of node %v in retry %d/%d, but got error: %v\n", node.Name, i, retryOnStatusConflict, updateErr)
		}

		// Try to update node on Kubernetes API server
		newNode, getErr := client.Nodes().Get(node.Name)
		if getErr != nil {
			return fmt.Errorf("Tried to update status of node %v in retry %d/%d, but got error: %v\n", node.Name, i, retryOnStatusConflict, getErr)
		}

		// Use newer version from API server, chanage labels and try to update again
		node = *newNode
		fmt.Printf("Tried to update status of node %v in retry %d/%d, but encountered status error (%v), retrying\n", node.Name, i, retryOnStatusConflict, statusErr)
	}
	return nil
}

// getClient returns a Kubernetes client
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

// Reset sets the joule labels of all nodes to the value previously assigned by /setup
func reset(w http.ResponseWriter, r *http.Request) {
	nodeClient := client.Nodes()

	if len(lastLabels) == 0 {
		fmt.Printf("No last labels map, run /setup first\n")
		return
	}
	for name, joules := range lastLabels {
		node, err := nodeClient.Get(name)
		if err != nil {
			fmt.Printf("Failed to get node '%s', skipping...\n", name)
		}
		node.Labels[joulesLabelName] = joules
		nodeClient.Update(node)
		fmt.Printf("updated node %s label: %s=%s\n", node.Name, joulesLabelName, joules)
	}
}

// normFloat returns a float value between 0 and 100 from the normal distribution
func normFloat(stdIn, meanIn float64) float64 {
	pick := math.MaxFloat64
	for pick > 100.0 || pick < 0.0 {
		pick = rand.NormFloat64()*stdIn + meanIn
	}
	return pick
}

// parseFloatInto tries to parse source string to a float. If the conversion succeeds
// dest is overwritten with the parsed value, otherwise it is left untouched
func parseFloatInto(dest *float64, source string) {
	parsed, err := strconv.ParseFloat(source, 64)
	if err != nil {
		fmt.Printf("error converting string '%v' to float, using default\n", source)
		return
	}
	*dest = parsed
}
