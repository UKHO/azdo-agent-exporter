package main

import (
	"io/ioutil"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/prometheus/client_golang/prometheus"

	"ukho.gov.uk/azdo-agent-exporter/azdo"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (transport roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestCollectUsesCachedMetrics(t *testing.T) {
	requests := 0
	failurePath := ""
	collector := newAzDoCollector(azdo.AzDoClient{
		Address: "http://azdo.test",
		Client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requests++
			body := `{"value":[]}`
			switch {
			case strings.HasSuffix(request.URL.Path, "/pools"):
				body = `{"value":[{"id":1,"name":"pool"}]}`
			case strings.HasSuffix(request.URL.Path, "/agents"):
				body = `{"value":[{"enabled":true,"status":"online"}]}`
			}
			if failurePath != "" && strings.Contains(request.URL.Path, failurePath) {
				body = "invalid JSON"
			}
			return &http.Response{StatusCode: http.StatusOK, Body: ioutil.NopCloser(strings.NewReader(body))}, nil
		})},
	}, true)
	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(collector)
	if requests != 0 {
		t.Fatal("registration made upstream requests")
	}
	empty, err := registry.Gather()
	if err != nil || len(empty) != 0 || requests != 0 {
		t.Fatalf("initial gather: metrics=%d requests=%d error=%v", len(empty), requests, err)
	}
	collector.refresh()
	if requests != 3 {
		t.Fatalf("refresh made %d requests, want 3", requests)
	}
	before, err := registry.Gather()
	if err != nil || len(before) != 8 {
		t.Fatalf("gather after refresh: metrics=%d error=%v", len(before), err)
	}
	after, err := registry.Gather()
	if err != nil || !reflect.DeepEqual(before, after) || requests != 3 {
		t.Fatalf("cached gather changed metrics or made requests: requests=%d error=%v", requests, err)
	}
	lastScrape := collector.lastScrape
	for _, path := range []string{"/pools", "/agents", "/jobrequests"} {
		failurePath = path
		collector.refresh()
		after, err = registry.Gather()
		if err != nil || !reflect.DeepEqual(before, after) || collector.lastScrape != lastScrape {
			t.Fatalf("failed refresh at %s changed snapshot or watermark: %v", path, err)
		}
	}
	failurePath = ""
	collector.refresh()
	if !collector.lastScrape.After(lastScrape) {
		t.Fatal("successful refresh did not advance watermark")
	}
}

func TestCollectDoesNotWaitForRefresh(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	collector := newAzDoCollector(azdo.AzDoClient{
		Address: "http://azdo.test",
		Client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			close(started)
			<-release
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       ioutil.NopCloser(strings.NewReader(`{"value":[]}`)),
			}, nil
		})},
	}, true)
	collector.cachedMetrics = []prometheus.Metric{
		prometheus.MustNewConstMetric(installedBuildAgentsDurationDesc, prometheus.GaugeValue, 1),
	}
	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)
	go func() {
		defer close(finished)
		collector.refresh()
	}()
	defer func() {
		close(release)
		<-finished
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("refresh did not start")
	}
	gathered := make(chan error, 1)
	go func() {
		_, err := registry.Gather()
		gathered <- err
	}()
	select {
	case err := <-gathered:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("gather blocked on upstream refresh")
	}
}

func TestRefreshIntervalConfig(t *testing.T) {
	var configuration config
	if _, err := toml.Decode("[exporter]\nrefreshInterval = \"10m\"", &configuration); err != nil {
		t.Fatal(err)
	}
	if configuration.Exporter.RefreshInterval != 10*time.Minute {
		t.Fatalf("decoded interval = %s, want 10m", configuration.Exporter.RefreshInterval)
	}
	if refreshIntervalDefault != 5*time.Minute {
		t.Fatalf("default interval = %s, want 5m", refreshIntervalDefault)
	}
}
