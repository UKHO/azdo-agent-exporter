package main

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	"ukho.gov.uk/azdo-agent-exporter/azdo"
)

type azDoCollector struct {
	AzDoClient        *azdo.AzDoClient
	ignoreHostedPools bool
	lastScrape        time.Time
	refreshMu         sync.Mutex
	metricsMu         sync.RWMutex
	cachedMetrics     []prometheus.Metric
}

func newAzDoCollector(az azdo.AzDoClient, ignoreHostedPools bool) *azDoCollector {
	return &azDoCollector{AzDoClient: &az, ignoreHostedPools: ignoreHostedPools}
}

func (azc *azDoCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- installedBuildAgentsDesc
	ch <- installedBuildAgentsDurationDesc
	ch <- totalJobsDesc
	ch <- queuedJobsDesc
	ch <- runningJobsDesc
	for _, metric := range calculateHistograms(metricsContext{}) {
		ch <- metric.Desc()
	}
}

func (azc *azDoCollector) Collect(publishMetrics chan<- prometheus.Metric) {
	azc.metricsMu.RLock()
	metrics := azc.cachedMetrics
	azc.metricsMu.RUnlock()

	for _, metric := range metrics {
		publishMetrics <- metric
	}
}

func (azc *azDoCollector) startRefresh(interval time.Duration) {
	if interval <= 0 {
		interval = refreshIntervalDefault
	}
	go func() {
		azc.refresh()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			azc.refresh()
		}
	}()
}

func (azc *azDoCollector) refresh() {
	azc.refreshMu.Lock()
	defer azc.refreshMu.Unlock()

	start := time.Now()
	pools, err := azc.AzDoClient.Pools(azc.ignoreHostedPools)
	if err != nil {
		log.WithFields(log.Fields{"serverName": azc.AzDoClient.Name, "error": err}).Error("Refresh failed to retrieve pools")
		return
	}

	metrics := []prometheus.Metric{}
	for _, pool := range pools {
		agents, err := azc.AzDoClient.Agents(pool.ID)
		if err != nil {
			log.WithFields(log.Fields{
				"serverName": azc.AzDoClient.Name, "poolId": pool.ID, "error": err,
			}).Error("Refresh failed to retrieve agents")
			return
		}

		finishedJobs, currentJobs, err := azc.AzDoClient.JobsAfter(pool.ID, azc.lastScrape)
		if err != nil {
			log.WithFields(log.Fields{
				"serverName": azc.AzDoClient.Name, "poolId": pool.ID, "error": err,
			}).Error("Refresh failed to retrieve jobs")
			return
		}

		metricContext := metricsContext{pool: pool, agents: agents, currentJobs: currentJobs, finishedJobs: finishedJobs}
		metrics = append(metrics, calculateAgentMetrics(metricContext)...)
		metrics = append(metrics, calculateJobMetrics(metricContext)...)
	}

	metrics = append(metrics, prometheus.MustNewConstMetric(
		installedBuildAgentsDurationDesc,
		prometheus.GaugeValue,
		time.Since(start).Seconds(),
	))
	azc.metricsMu.Lock()
	azc.cachedMetrics = metrics
	azc.metricsMu.Unlock()
	azc.lastScrape = start
	log.WithField("serverName", azc.AzDoClient.Name).Info("Refreshed agents")
}

type metricsContext struct {
	pool         azdo.Pool
	agents       []azdo.Agent
	currentJobs  []azdo.Job
	finishedJobs []azdo.Job
}
