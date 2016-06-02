package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	schedulerapi "k8s.io/kubernetes/plugin/pkg/scheduler/api"

	"k8s.io/kubernetes/pkg/api"
)

// newNode returns a new api.Node given a name and joules.
func newNode(name string, joules string) api.Node {
	jmap := make(map[string]string)
	if joules != "" {
		jmap["joules"] = joules
	}
	return api.Node{
		ObjectMeta: api.ObjectMeta{
			Name:   name,
			Labels: jmap,
		},
	}
}

// newNodeList returns a node list.
func newNodeList(nodes ...api.Node) api.NodeList {
	return api.NodeList{
		Items: nodes,
	}
}

// TestSelectNodeSuccess tests selectNode using different inputs.
func TestSelectNodeSuccess(t *testing.T) {
	testCases := map[string]struct {
		list     api.NodeList
		expected string
	}{
		"sorted": {
			list: newNodeList(
				newNode("node1", "50.5"),
				newNode("node2", "70.5"),
				newNode("node3", "80.5"),
			),
			expected: "node1",
		},
		"reverse sorted": {
			list: newNodeList(
				newNode("node1", "80.5"),
				newNode("node2", "70.5"),
				newNode("node3", "50.5"),
			),
			expected: "node3",
		},
		"mixed": {
			list: newNodeList(
				newNode("node1", "80.5"),
				newNode("node2", "50.5"),
				newNode("node3", "70.5"),
			),
			expected: "node2",
		},
		"illigal joules": {
			list: newNodeList(
				newNode("node1", "55.5"),
				newNode("node2", "65.5"),
				newNode("node3", "illigal"),
			),
			expected: "node1",
		},
		"no joules": {
			list: newNodeList(
				newNode("node1", "55.5"),
				newNode("node2", "65.5"),
				newNode("node3", ""),
			),
			expected: "node1",
		},
	}

	for desc, tc := range testCases {
		node, err := selectNode(&tc.list)
		if err != nil {
			t.Errorf("Error when testing case %v: %v", desc, err)
		} else {
			if node.Name != tc.expected {
				t.Errorf("Test case %v: expected %v but got %v", desc, tc.expected, node.Name)
			}
		}
	}
}

// TestSelectNodeFail tests the case when selecting a node fails.
func TestSelectNodeFail(t *testing.T) {
	list := newNodeList()
	_, err := selectNode(&list)
	if err == nil {
		t.Errorf("Expected error because list was empty")
	}
}

// TestHandler tests the handler function.
func TestHandler(t *testing.T) {
	// New test server
	srv := httptest.NewServer(http.HandlerFunc(handler))
	defer srv.Close()

	// Input to send for test and expected value
	expected := "node1"
	args := &schedulerapi.ExtenderArgs{
		Pod: api.Pod{},
		Nodes: newNodeList(
			newNode("node1", "50.5"),
			newNode("node2", "70.5"),
			newNode("node3", "80.5"),
		),
	}

	// convert to json
	b, err := json.Marshal(args)
	if err != nil {
		t.Errorf("Error when trying to convert args to bytes: %v", err)
		return
	}
	// send request to fake server
	res, err := http.Post(srv.URL, "application/json", bytes.NewBuffer(b))
	if err != nil {
		t.Errorf("Error when making post request: %v", err)
		return
	}

	// decode result from fake server
	dec := json.NewDecoder(res.Body)
	received := &schedulerapi.ExtenderFilterResult{}
	err = dec.Decode(received)
	if err != nil {
		t.Errorf("Error when trying to convert result to ExtenderFilterResult: %v", err)
		return
	}

	// handle result
	if len(received.Nodes.Items) != 1 {
		t.Errorf("Expected one node but received %v", len(received.Nodes.Items))
		return
	}
	if received.Nodes.Items[0].Name != expected {
		t.Errorf("Expected node %v to be scheduled but %v was chosen", expected, received.Nodes.Items[0].Name)
	}
}
