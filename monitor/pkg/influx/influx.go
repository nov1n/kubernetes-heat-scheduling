package influx

import (
	"fmt"
	"time"

	"github.com/influxdata/influxdb/client/v2"
)

const (
	service   = "http://nce-pm-influxdb.default"
	db        = "k8s"
	port      = "8086"
	username  = "root"
	password  = "root"
	hostLabel = "hostname"
)

// Client is responsible for inserting Points into InfluxDB
type Client struct {
	HTTP client.Client
}

// NewClient returns a new Influx object
func NewClient() (*Client, error) {
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
