package vlselect

import (
	"flag"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/VictoriaMetrics/VictoriaMetrics/app/vlselect/logsql"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/cgroup"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/httpserver"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/httputils"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/logger"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/timerpool"
	"github.com/VictoriaMetrics/metrics"
)

var (
	maxConcurrentRequests = flag.Int("search.maxConcurrentRequests", getDefaultMaxConcurrentRequests(), "The maximum number of concurrent search requests. "+
		"It shouldn't be high, since a single request can saturate all the CPU cores, while many concurrently executed requests may require high amounts of memory. "+
		"See also -search.maxQueueDuration")
	maxQueueDuration = flag.Duration("search.maxQueueDuration", 10*time.Second, "The maximum time the search request waits for execution when -search.maxConcurrentRequests "+
		"limit is reached; see also -search.maxQueryDuration")
	maxQueryDuration = flag.Duration("search.maxQueryDuration", time.Second*30, "The maximum duration for query execution")
)

func getDefaultMaxConcurrentRequests() int {
	n := cgroup.AvailableCPUs()
	if n <= 4 {
		n *= 2
	}
	if n > 16 {
		// A single request can saturate all the CPU cores, so there is no sense
		// in allowing higher number of concurrent requests - they will just contend
		// for unavailable CPU time.
		n = 16
	}
	return n
}

// Init initializes vlselect
func Init() {
	concurrencyLimitCh = make(chan struct{}, *maxConcurrentRequests)
}

// Stop stops vlselect
func Stop() {
}

var concurrencyLimitCh chan struct{}

var (
	concurrencyLimitReached = metrics.NewCounter(`vl_concurrent_select_limit_reached_total`)
	concurrencyLimitTimeout = metrics.NewCounter(`vl_concurrent_select_limit_timeout_total`)

	_ = metrics.NewGauge(`vl_concurrent_select_capacity`, func() float64 {
		return float64(cap(concurrencyLimitCh))
	})
	_ = metrics.NewGauge(`vl_concurrent_select_current`, func() float64 {
		return float64(len(concurrencyLimitCh))
	})
)

// RequestHandler handles select requests for VictoriaLogs
func RequestHandler(w http.ResponseWriter, r *http.Request) bool {
	path := r.URL.Path
	if !strings.HasPrefix(path, "/select/") {
		return false
	}
	path = strings.TrimPrefix(path, "/select")
	path = strings.ReplaceAll(path, "//", "/")

	// Limit the number of concurrent queries.
	startTime := time.Now()
	stopCh := r.Context().Done()
	select {
	case concurrencyLimitCh <- struct{}{}:
		defer func() { <-concurrencyLimitCh }()
	default:
		// Sleep for a while until giving up. This should resolve short bursts in requests.
		concurrencyLimitReached.Inc()
		d := getMaxQueryDuration(r)
		if d > *maxQueueDuration {
			d = *maxQueueDuration
		}
		t := timerpool.Get(d)
		select {
		case concurrencyLimitCh <- struct{}{}:
			timerpool.Put(t)
			defer func() { <-concurrencyLimitCh }()
		case <-stopCh:
			timerpool.Put(t)
			remoteAddr := httpserver.GetQuotedRemoteAddr(r)
			requestURI := httpserver.GetRequestURI(r)
			logger.Infof("client has cancelled the request after %.3f seconds: remoteAddr=%s, requestURI: %q",
				time.Since(startTime).Seconds(), remoteAddr, requestURI)
			return true
		case <-t.C:
			timerpool.Put(t)
			concurrencyLimitTimeout.Inc()
			err := &httpserver.ErrorWithStatusCode{
				Err: fmt.Errorf("couldn't start executing the request in %.3f seconds, since -search.maxConcurrentRequests=%d concurrent requests "+
					"are executed. Possible solutions: to reduce query load; to add more compute resources to the server; "+
					"to increase -search.maxQueueDuration=%s; to increase -search.maxQueryDuration; to increase -search.maxConcurrentRequests",
					d.Seconds(), *maxConcurrentRequests, maxQueueDuration),
				StatusCode: http.StatusServiceUnavailable,
			}
			httpserver.Errorf(w, r, "%s", err)
			return true
		}
	}

	switch {
	case path == "/logsql/query":
		logsqlQueryRequests.Inc()
		httpserver.EnableCORS(w, r)
		logsql.ProcessQueryRequest(w, r, stopCh)
		return true
	default:
		return false
	}
}

// getMaxQueryDuration returns the maximum duration for query from r.
func getMaxQueryDuration(r *http.Request) time.Duration {
	dms, err := httputils.GetDuration(r, "timeout", 0)
	if err != nil {
		dms = 0
	}
	d := time.Duration(dms) * time.Millisecond
	if d <= 0 || d > *maxQueryDuration {
		d = *maxQueryDuration
	}
	return d
}

var (
	logsqlQueryRequests = metrics.NewCounter(`vl_http_requests_total{path="/select/logsql/query"}`)
)