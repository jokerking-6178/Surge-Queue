package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// Load tester for the Surge Protection Queue System demo.
//
// Two modes:
//   - "with_queue": users enter the queue, poll status, and only hit the
//     backend once they're admitted. This shows the system staying smooth.
//   - "without_queue": users slam the backend directly. This shows the
//     backend degrading or erroring under burst load.
//
// Usage:
//
//	# With queue (protected)
//	go run . --mode=with-queue --users=2000 --ramp=2s --queue=http://localhost:8080 --backend=http://localhost:8081
//
//	# Without queue (unprotected — chaos)
//	go run . --mode=without-queue --users=2000 --ramp=2s --backend=http://localhost:8081

var (
	mode    = flag.String("mode", "with-queue", "with-queue | without-queue")
	users   = flag.Int("users", 1000, "total virtual users to simulate")
	rampDur = flag.Duration("ramp", 5*time.Second, "ramp-up duration over which users are launched")
	queue   = flag.String("queue", "http://localhost:8080", "queue engine URL")
	backend = flag.String("backend", "http://localhost:8081", "backend URL")
)

// Stats collected across all virtual users.
type Stats struct {
	entered      int64
	admitted     int64
	completed    int64
	failed       int64
	backend200   int64
	backend5xx   int64
	totalWaitMs  int64
	maxWaitMs    int64
	startTime    time.Time
}

func main() {
	flag.Parse()
	fmt.Printf("=== Surge Queue Load Test ===\n")
	fmt.Printf("  mode:     %s\n", *mode)
	fmt.Printf("  users:    %d\n", *users)
	fmt.Printf("  ramp:     %s\n", *rampDur)
	fmt.Printf("  queue:    %s\n", *queue)
	fmt.Printf("  backend:  %s\n", *backend)
	fmt.Println()

	stats := &Stats{startTime: time.Now()}

	// Print a live stats ticker
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				printLiveStats(stats)
			case <-done:
				return
			}
		}
	}()

	// Launch users with ramp-up
	var wg sync.WaitGroup
	rampInterval := *rampDur / time.Duration(*users)
	if rampInterval < time.Millisecond {
		rampInterval = time.Millisecond
	}

	for i := 0; i < *users; i++ {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()
			simulateUser(stats, fmt.Sprintf("loadtest-%d-%d", os.Getpid(), userID))
		}(i)
		time.Sleep(rampInterval)
	}

	wg.Wait()
	close(done)

	fmt.Println("\n=== Final Results ===")
	printFinalStats(stats)
}

// simulateUser simulates a single user going through the flow.
func simulateUser(stats *Stats, userID string) {
	switch *mode {
	case "with-queue":
		simulateWithQueue(stats, userID)
	case "without-queue":
		simulateWithoutQueue(stats, userID)
	}
}

// simulateWithQueue: enter queue → poll → get admitted → register.
func simulateWithQueue(stats *Stats, userID string) {
	client := &http.Client{Timeout: 10 * time.Second}

	// 1. Enter the queue
	enterURL := fmt.Sprintf("%s/api/queue/enter", *queue)
	body := fmt.Sprintf(`{"user_id":"%s"}`, userID)
	resp, err := client.Post(enterURL, "application/json", strReader(body))
	if err != nil {
		atomic.AddInt64(&stats.failed, 1)
		return
	}
	resp.Body.Close()
	atomic.AddInt64(&stats.entered, 1)

	// 2. Poll until admitted
	pollStart := time.Now()
	maxWait := 5 * time.Minute
	for {
		if time.Since(pollStart) > maxWait {
			atomic.AddInt64(&stats.failed, 1)
			return
		}

		statusURL := fmt.Sprintf("%s/api/queue/status/%s", *queue, userID)
		resp, err := client.Get(statusURL)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		var result struct {
			State      string `json:"state"`
			Position   int64  `json:"position"`
			AdmitToken string `json:"admit_token"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()

		if result.State == "admitted" && result.AdmitToken != "" {
			waitMs := time.Since(pollStart).Milliseconds()
			atomic.AddInt64(&stats.admitted, 1)
			atomic.AddInt64(&stats.totalWaitMs, waitMs)
			updateMax(&stats.maxWaitMs, waitMs)

			// 3. Hit the backend with the admit token
			registerWithToken(stats, userID, result.AdmitToken)
			return
		}

		// Poll every 1-2 seconds (simulates realistic client behavior)
		time.Sleep(time.Duration(1000+rand.Intn(1000)) * time.Millisecond)
	}
}

// simulateWithoutQueue: slam the backend directly — no queue protection.
func simulateWithoutQueue(stats *Stats, userID string) {
	registerDirect(stats, userID)
}

func registerWithToken(stats *Stats, userID, token string) {
	client := &http.Client{Timeout: 10 * time.Second}
	body := fmt.Sprintf(`{"name":"User %s","email":"%s@test.com","details":"load test"}`, userID, userID)
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/register", *backend), strReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		atomic.AddInt64(&stats.failed, 1)
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == 200 {
		atomic.AddInt64(&stats.backend200, 1)
		atomic.AddInt64(&stats.completed, 1)
	} else if resp.StatusCode >= 500 {
		atomic.AddInt64(&stats.backend5xx, 1)
		atomic.AddInt64(&stats.failed, 1)
	}
}

func registerDirect(stats *Stats, userID string) {
	client := &http.Client{Timeout: 10 * time.Second}
	body := fmt.Sprintf(`{"name":"User %s","email":"%s@test.com","details":"load test no queue"}`, userID, userID)
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/register", *backend), strReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		atomic.AddInt64(&stats.failed, 1)
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == 200 {
		atomic.AddInt64(&stats.backend200, 1)
		atomic.AddInt64(&stats.completed, 1)
	} else if resp.StatusCode >= 500 {
		atomic.AddInt64(&stats.backend5xx, 1)
		atomic.AddInt64(&stats.failed, 1)
	}
}

// --- helpers ---

func strReader(s string) io.Reader {
	return &stringReader{s: s}
}

type stringReader struct {
	s string
	i int
}

func (r *stringReader) Read(p []byte) (int, error) {
	if r.i >= len(r.s) {
		return 0, io.EOF
	}
	n := copy(p, r.s[r.i:])
	r.i += n
	return n, nil
}

func updateMax(target *int64, val int64) {
	for {
		old := atomic.LoadInt64(target)
		if val <= old {
			return
		}
		if atomic.CompareAndSwapInt64(target, old, val) {
			return
		}
	}
}

func printLiveStats(stats *Stats) {
	elapsed := time.Since(stats.startTime).Seconds()
	entered := atomic.LoadInt64(&stats.entered)
	admitted := atomic.LoadInt64(&stats.admitted)
	completed := atomic.LoadInt64(&stats.completed)
	failed := atomic.LoadInt64(&stats.failed)
	avgWait := int64(0)
	if admitted > 0 {
		avgWait = atomic.LoadInt64(&stats.totalWaitMs) / admitted
	}
	maxWait := atomic.LoadInt64(&stats.maxWaitMs)
	b200 := atomic.LoadInt64(&stats.backend200)
	b5xx := atomic.LoadInt64(&stats.backend5xx)

	fmt.Printf("[%6.1fs] entered=%d admitted=%d completed=%d failed=%d | avg_wait=%.1fs max_wait=%.1fs | 2xx=%d 5xx=%d\n",
		elapsed, entered, admitted, completed, failed,
		float64(avgWait)/1000, float64(maxWait)/1000, b200, b5xx)
}

func printFinalStats(stats *Stats) {
	elapsed := time.Since(stats.startTime).Seconds()
	entered := atomic.LoadInt64(&stats.entered)
	admitted := atomic.LoadInt64(&stats.admitted)
	completed := atomic.LoadInt64(&stats.completed)
	failed := atomic.LoadInt64(&stats.failed)
	avgWait := int64(0)
	if admitted > 0 {
		avgWait = atomic.LoadInt64(&stats.totalWaitMs) / admitted
	}
	maxWait := atomic.LoadInt64(&stats.maxWaitMs)
	b200 := atomic.LoadInt64(&stats.backend200)
	b5xx := atomic.LoadInt64(&stats.backend5xx)

	fmt.Printf("Duration:         %.1fs\n", elapsed)
	fmt.Printf("Users entered:    %d\n", entered)
	fmt.Printf("Users admitted:   %d\n", admitted)
	fmt.Printf("Registrations:    %d (2xx=%d, 5xx=%d)\n", completed, b200, b5xx)
	fmt.Printf("Failed:           %d\n", failed)
	fmt.Printf("Avg wait time:    %.1fs\n", float64(avgWait)/1000)
	fmt.Printf("Max wait time:    %.1fs\n", float64(maxWait)/1000)

	if b5xx > 0 && *mode == "without-queue" {
		log.Printf("\n>>> Without queue: %d requests got 5xx errors — backend degraded under burst load!", b5xx)
	}
	if *mode == "with-queue" && failed == 0 {
		log.Printf("\n>>> With queue: all %d users processed smoothly, zero errors.", completed)
	}
}
