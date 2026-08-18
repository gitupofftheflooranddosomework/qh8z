package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	target := flag.String("url", "", "URL to load test")
	duration := flag.Duration("duration", 30*time.Second, "test duration")
	concurrency := flag.Int("concurrency", 20, "concurrent workers")
	expectedStatus := flag.Int("expect-status", http.StatusFound, "expected HTTP status")
	maxErrorRate := flag.Float64("max-error-rate", 0.01, "maximum tolerated error ratio")
	maxP95 := flag.Duration("max-p95", 500*time.Millisecond, "maximum tolerated p95 latency")
	flag.Parse()

	if *target == "" || *duration <= 0 || *concurrency < 1 {
		flag.Usage()
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	var total atomic.Uint64
	var failed atomic.Uint64
	var latencyMu sync.Mutex
	latencies := make([]time.Duration, 0, 100000)

	worker := func() {
		for ctx.Err() == nil {
			started := time.Now()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, *target, nil)
			if err != nil {
				failed.Add(1)
				total.Add(1)
				continue
			}
			resp, err := client.Do(req)
			latency := time.Since(started)
			total.Add(1)
			if err != nil {
				if ctx.Err() == nil {
					failed.Add(1)
				}
			} else {
				_ = resp.Body.Close()
				if resp.StatusCode != *expectedStatus {
					failed.Add(1)
				}
			}
			latencyMu.Lock()
			if len(latencies) < cap(latencies) {
				latencies = append(latencies, latency)
			}
			latencyMu.Unlock()
		}
	}

	var wg sync.WaitGroup
	wg.Add(*concurrency)
	for range *concurrency {
		go func() {
			defer wg.Done()
			worker()
		}()
	}
	wg.Wait()

	requests := total.Load()
	failures := failed.Load()
	if requests == 0 {
		fmt.Fprintln(os.Stderr, "load test made no requests")
		os.Exit(1)
	}
	latencyMu.Lock()
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50 := percentile(latencies, 0.50)
	p95 := percentile(latencies, 0.95)
	p99 := percentile(latencies, 0.99)
	latencyMu.Unlock()
	errorRate := float64(failures) / float64(requests)
	rps := float64(requests) / duration.Seconds()

	fmt.Printf("requests=%d failures=%d error_rate=%.4f rps=%.1f p50=%s p95=%s p99=%s\n", requests, failures, errorRate, rps, p50, p95, p99)
	if errorRate > *maxErrorRate {
		fmt.Fprintf(os.Stderr, "error rate %.4f exceeded %.4f\n", errorRate, *maxErrorRate)
		os.Exit(1)
	}
	if p95 > *maxP95 {
		fmt.Fprintf(os.Stderr, "p95 %s exceeded %s\n", p95, *maxP95)
		os.Exit(1)
	}
}

func percentile(values []time.Duration, quantile float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	index := int(float64(len(values)-1) * quantile)
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}
