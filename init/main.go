package main

import (
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"sync/atomic"

	"k8s.io/kubernetes/pkg/api"
	k8sApiErr "k8s.io/kubernetes/pkg/api/errors"
	k8sRestCl "k8s.io/kubernetes/pkg/client/restclient"
	k8sClient "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/fields"
	"k8s.io/kubernetes/pkg/labels"
)

const (
	k8sHost               = "127.0.0.1"
	k8sPort               = "8080"
	retryOnStatusConflict = 3
	port                  = "8090"
	joulesLabelName       = "joules"
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
	http.HandleFunc("/", hello)
	http.HandleFunc("/setup", setup)
	http.HandleFunc("/reset", reset)
	fmt.Printf("Listening on localhost%v", port)
	http.ListenAndServe(":"+port, nil)
}

func hello(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello!")
	return
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

	nodeClient := client.Nodes()

	listAll := api.ListOptions{LabelSelector: labels.Everything(), FieldSelector: fields.Everything()}
	nodes, err := nodeClient.List(listAll)
	if err != nil {
		fmt.Printf("Could not list nodes: %v\n", err)
		return
	}

	lastLabels = make(map[string]string)
	var counter int64 = 1
	for _, node := range nodes.Items {
		go func(node api.Node) {
			// Log when done
			defer func() {
				fmt.Printf("%v/%v: updated node %s\n", counter, len(nodes.Items), node.Name)
				atomic.AddInt64(&counter, 1)
			}()

			// Compute joule value
			joules := normFloat(std, mean)
			joulesString := fmt.Sprintf("%.2f", joules) // Truncate at two decimals
			// Retry on status conflict
			for i := 0; ; i++ {
				node.Labels[joulesLabelName] = joulesString
				// Update node
				_, updateErr := nodeClient.Update(&node)
				if updateErr == nil {
					break
				}
				if i >= retryOnStatusConflict {
					fmt.Printf("Tried to update status of node %v, but amount of retries (%d) exceeded\n", node.Name, retryOnStatusConflict)
					return
				}
				statusErr, ok := updateErr.(*k8sApiErr.StatusError)
				if !ok {
					fmt.Printf("Tried to update status of node %v in retry %d/%d, but got error: %v\n", node.Name, i, retryOnStatusConflict, updateErr)
					return
				}
				newNode, getErr := nodeClient.Get(node.Name)
				if getErr != nil {
					fmt.Printf("Tried to update status of node %v in retry %d/%d, but got error: %v\n", node.Name, i, retryOnStatusConflict, getErr)
					return
				}

				// Use newer version from API server, chanage labels and try to update again
				node = *newNode
				fmt.Printf("Tried to update status of node %v in retry %d/%d, but encountered status error (%v), retrying\n", node.Name, i, retryOnStatusConflict, statusErr)
			}

			// Update lastlabels to allow resets
			lastLabels[node.Name] = joulesString
		}(node)
	}
}

// getClient returns a kubernetes client
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

// logAndWrite writes to stdout and to the provided writer
func logAndWrite(w io.Writer, template string, vars ...interface{}) {
	fmt.Fprintf(w, template, vars...)
	fmt.Printf(template, vars...)
}
