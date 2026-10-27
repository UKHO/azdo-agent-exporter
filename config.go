package main

import (
	"net/url"
	"time"

	"./azdo"
)

var (
	portDefault            = 8080
	endpointDefault        = "/metrics"
	refreshIntervalDefault = 5 * time.Minute
)

type config struct {
	Servers  map[string]azDoConfig
	Proxy    proxy
	Exporter exporter
}

type exporter struct {
	Port            int
	Endpoint        string
	RefreshInterval time.Duration
}

type proxy struct {
	URL      string
	proxyURL *url.URL
}

type azDoConfig struct {
	azdo.AzDoClient
	UseProxy bool
}
