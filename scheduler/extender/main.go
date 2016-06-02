package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"

	"k8s.io/kubernetes/pkg/api"
	schedulerapi "k8s.io/kubernetes/plugin/pkg/scheduler/api"
)

func handler(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(r.Body)
	received := &schedulerapi.ExtenderArgs{}
	err := dec.Decode(received)
	if err != nil {
		fmt.Printf("Error when trying to decode response body to struct: %v\n", err)
		return
	}

	for _, n := range received.Nodes.Items {
		fmt.Printf("Received node %v with joules %v\n", n.Name, n.Labels["joules"])
	}

	node, err := selectNode(&received.Nodes)
	var items []api.Node
	if err != nil {
		// glog errorf
	} else {
		items = []api.Node{*node}
	}

	enc := json.NewEncoder(w)
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	enc.Encode(&schedulerapi.ExtenderFilterResult{
		Nodes: api.NodeList{
			Items: items,
		},
	})
	fmt.Printf("Chose node %v (joules=%v) for pod %v\n", node.Name, node.Labels["joules"], received.Pod.Name)
	return
}

func selectNode(nodes *api.NodeList) (*api.Node, error) {
	if len(nodes.Items) == 0 {
		return nil, fmt.Errorf("No nodes were provided")
	}

	// find min joules value
	min := math.MaxFloat64
	for _, node := range nodes.Items {
		min = math.Min(min, jouleFromLabels(&node))
	}

	// find node belonging to min joules value
	for _, node := range nodes.Items {
		if min == jouleFromLabels(&node) {
			return &node, nil
		}
	}

	return nil, fmt.Errorf("No suitable nodes found.")
}

func jouleFromLabels(node *api.Node) float64 {
	jouleString, exists := node.Labels["joules"]
	if exists {
		joule, err := strconv.ParseFloat(jouleString, 32)
		if err == nil {
			return joule
		}
	}
	return math.MaxFloat64
}

func main() {
	// flag.Parse()

	fmt.Printf("Starts listening\n")

	svr := &http.Server{
		Addr: ":8100",
	}
	svr.Handler = http.HandlerFunc(handler)

	svr.ListenAndServe()
	ch := make(chan bool)
	<-ch
}
