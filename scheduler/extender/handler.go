package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	k8sApi "k8s.io/kubernetes/pkg/api"
	k8sSchedulerApi "k8s.io/kubernetes/plugin/pkg/scheduler/api"
)

// handler handles a request from the kubernetes scheduler.
// handler receives a list of nodes and a pod and returns the node with the
// lowest joules label.
func handler(w http.ResponseWriter, r *http.Request) {
	// decode request body.
	dec := json.NewDecoder(r.Body)
	received := &k8sSchedulerApi.ExtenderArgs{}
	err := dec.Decode(received)
	if err != nil {
		fmt.Printf("Error when trying to decode response body to struct: %v\n", err)
		return
	}

	logNodes(&received.Nodes)

	// select the node to schedule on.
	node, err := selectNode(&received.Nodes)
	var items []k8sApi.Node
	if err != nil {
		fmt.Printf("Encountered error when selecting node: %v", err)
	} else {
		items = []k8sApi.Node{*node}
	}

	// return the result.
	enc := json.NewEncoder(w)
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	enc.Encode(&k8sSchedulerApi.ExtenderFilterResult{
		Nodes: k8sApi.NodeList{
			Items: items,
		},
	})
	fmt.Printf("Chose node %v (joules=%v) for pod %v\n", node.Name, node.Labels["joules"], received.Pod.Name)
	return
}
