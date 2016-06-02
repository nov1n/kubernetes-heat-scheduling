package main

import (
	"encoding/json"
	"flag"
	"net/http"

	"github.com/golang/glog"

	"k8s.io/kubernetes/plugin/pkg/scheduler/api"
)

func handler(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(r.Body)
	result := &api.ExtenderArgs{}
	err := dec.Decode(result)
	if err != nil {
		glog.Errorf("Error when trying to decode response body to struct: %v", err)
		return
	}
	glog.Infof("Result: %#v", result)
	return
}

func main() {
	flag.Parse()

	http.HandleFunc("/filter", handler)
	http.ListenAndServe(":8080", nil)
}
