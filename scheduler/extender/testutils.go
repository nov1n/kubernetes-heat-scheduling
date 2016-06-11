package main

import "k8s.io/kubernetes/pkg/api"

// this file contains functions that are shared among both test files.

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
