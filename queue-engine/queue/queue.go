package queue

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/surge/queue-engine/config"
	"github.com/surge/queue-engine/jwt"
	"github.com/surge/queue-engine/metrics"
)

// Manager owns all queue operations against Redis.
//
// Design:
//   - Waiting users live in a Redis sorted set (ZSET), scored by the
//     millisecond timestamp of their entry. This gives FIFO ordering.
//   - Admitted (active) sessions are tracked in a Redis set with a TTL per
//     member, so slots auto-free when sessions expire.
//   - A background admitter goroutine runs a rate-limited loop: every tick it
//     computes available slots (maxConcurrent - currently active) and admits
//     up to (admitRatePerSec) users from the front of the queue, capped by
//     available slots.
//   - The admitter also sweeps expired entries.
type Manager struct {
	rdb  *redis.Client
	cfg  config.Config
}

// NewManager creates a Manager and starts the background admitter.
func NewManager(rdb *redis.Client, cfg config.Config) *Manager {
	m := &Manager{rdb: rdb, cfg: cfg}
	go m.admitterLoop()
	go m.cleanupLoop()
	return m
}

// QueueStatus is returned to the client on every status poll.
type QueueStatus struct {
	State      string `json:"state"`        // "waiting" | "admitted" | "expired"
	Position  int64  `json:"position"`     // 1-based position in queue (0 if admitted)
	Total      int64  `json:"total_waiting"`
	AdmitToken string `json:"admit_token,omitempty"` // present only when admitted
	ETASeconds int   `json:"eta_seconds"`
}

// Enter adds a user to the queue. If there's an immediate free slot, they may
// be admitted on the next admitter tick (within ~1s). Returns their initial status.
func (m *Manager) Enter(ctx context.Context, userID string) (*QueueStatus, error) {
	score := float64(time.Now().UnixMilli())

	// Add to sorted set only if not already in it (NX flag via pipeline).
	pipe := m.rdb.TxPipeline()
	pipe.ZAdd(ctx, m.cfg.QueueName, redis.Z{Score: score, Member: userID})
	pipe.ZCard(ctx, m.cfg.QueueName)
	res, err := pipe.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("enter: redis: %w", err)
	}

	total := res[1].(*redis.IntCmd).Val()
	// Check if already admitted
	if m.isActive(ctx, userID) {
		token, _ := jwt.Sign(m.cfg.JWTSecret, userID, m.cfg.JWTTTL)
		return &QueueStatus{State: "admitted", AdmitToken: token, Total: total}, nil
	}

	pos, _ := m.rdb.ZRank(ctx, m.cfg.QueueName, userID).Result()
	metrics.EnteredTotal.Inc()
	metrics.QueueDepth.Set(float64(total))

	return &QueueStatus{
		State:      "waiting",
		Position:   pos + 1,
		Total:      total,
		ETASeconds: m.estimateETA(pos),
	}, nil
}

// Status returns the current position/state for a user.
func (m *Manager) Status(ctx context.Context, userID string) (*QueueStatus, error) {
	// Already admitted?
	if m.isActive(ctx, userID) {
		token, _ := jwt.Sign(m.cfg.JWTSecret, userID, m.cfg.JWTTTL)
		total, _ := m.rdb.ZCard(ctx, m.cfg.QueueName).Result()
		return &QueueStatus{State: "admitted", AdmitToken: token, Total: total}, nil
	}

	// Still waiting?
	rank, err := m.rdb.ZRank(ctx, m.cfg.QueueName, userID).Result()
	if err == redis.Nil {
		return &QueueStatus{State: "expired"}, nil
	}
	if err != nil {
		return nil, err
	}

	total, _ := m.rdb.ZCard(ctx, m.cfg.QueueName).Result()
	return &QueueStatus{
		State:      "waiting",
		Position:   rank + 1,
		Total:      total,
		ETASeconds: m.estimateETA(rank),
	}, nil
}

// Heartbeat refreshes the user's TTL in the waiting set so they don't get
// evicted while the tab is open.
func (m *Manager) Heartbeat(ctx context.Context, userID string) error {
	// Re-add updates the member (no-op if exists). We also touch a
	// heartbeat hash to track liveness for cleanup.
	_, err := m.rdb.HSet(ctx, "surge:heartbeats", userID, time.Now().Unix()).Result()
	return err
}

// ---- internal ----

func (m *Manager) estimateETA(rank int64) int {
	if rank < 0 {
		return 0
	}
	seconds := int(rank) / m.cfg.AdmitRatePerSec
	return seconds
}

func (m *Manager) isActive(ctx context.Context, userID string) bool {
	n, err := m.rdb.SIsMember(ctx, "surge:admitted", userID).Result()
	if err != nil {
		return false
	}
	return n
}

// activeCount returns how many admitted sessions are currently live.
func (m *Manager) activeCount(ctx context.Context) int64 {
	n, _ := m.rdb.SCard(ctx, "surge:admitted").Result()
	return n
}

// admittedKeyTTL is a Redis hash mapping userID -> expiry timestamp.
// We store this so the cleanup loop can remove expired entries.
func (m *Manager) markAdmitted(ctx context.Context, userID string) error {
	pipe := m.rdb.TxPipeline()
	pipe.ZRem(ctx, m.cfg.QueueName, userID)
	pipe.SAdd(ctx, "surge:admitted", userID)
	pipe.HSet(ctx, "surge:admitted_expiry", userID, time.Now().Add(time.Duration(m.cfg.AdmittedSessionTTL)*time.Second).Unix())
	_, err := pipe.Exec(ctx)
	return err
}

// admitterLoop is the rate-limited gate. Every tick (1/admitRate seconds, min 1s)
// it admits as many users as there are free slots, capped at admitRatePerSec.
func (m *Manager) admitterLoop() {
	tickerInterval := time.Second / time.Duration(m.cfg.AdmitRatePerSec)
	if tickerInterval < 100*time.Millisecond {
		// Don't tick faster than 10x/sec to avoid hammering Redis.
		tickerInterval = 100 * time.Millisecond
	}
	batchPerTick := int(time.Second / tickerInterval)
	if batchPerTick < 1 {
		batchPerTick = 1
	}

	ticker := time.NewTicker(tickerInterval)
	defer ticker.Stop()

	for range ticker.C {
		m.admitBatch(context.Background(), batchPerTick)
	}
}

func (m *Manager) admitBatch(ctx context.Context, max int) {
	active := m.activeCount(ctx)
	freeSlots := int64(m.cfg.MaxConcurrentAdmitted) - active
	if freeSlots <= 0 {
		return
	}

	toAdmit := int64(max)
	if toAdmit > freeSlots {
		toAdmit = freeSlots
	}

	// Pop from the front of the sorted set (lowest score = oldest).
	members, err := m.rdb.ZPopMin(ctx, m.cfg.QueueName, toAdmit).Result()
	if err != nil || len(members) == 0 {
		return
	}

	for _, z := range members {
		userID := z.Member.(string)
		enteredAt := time.UnixMilli(int64(z.Score))

		if err := m.markAdmitted(ctx, userID); err != nil {
			log.Printf("admitBatch: markAdmitted failed for %s: %v", userID, err)
			// Put them back at the front.
			m.rdb.ZAdd(ctx, m.cfg.QueueName, redis.Z{Score: z.Score, Member: userID})
			continue
		}

		elapsed := time.Since(enteredAt).Seconds()
		metrics.WaitTimeSeconds.Observe(elapsed)
		metrics.AdmittedTotal.Inc()

		log.Printf("admitted user %s (waited %.1fs)", userID, elapsed)
	}

	// Update gauges.
	remaining, _ := m.rdb.ZCard(ctx, m.cfg.QueueName).Result()
	metrics.QueueDepth.Set(float64(remaining))
	metrics.AdmittedActive.Set(float64(m.activeCount(ctx)))
}

// cleanupLoop removes expired admitted sessions and stale waiting entries.
func (m *Manager) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		m.cleanupExpiredAdmitted(context.Background())
		m.cleanupStaleWaiting(context.Background())
	}
}

func (m *Manager) cleanupExpiredAdmitted(ctx context.Context) {
	expiries, err := m.rdb.HGetAll(ctx, "surge:admitted_expiry").Result()
	if err != nil {
		return
	}
	now := time.Now().Unix()
	var expired []string
	for userID, expStr := range expiries {
		var exp int64
		fmt.Sscanf(expStr, "%d", &exp)
		if exp < now {
			expired = append(expired, userID)
		}
	}
	if len(expired) == 0 {
		return
	}
	pipe := m.rdb.TxPipeline()
	for _, uid := range expired {
		pipe.SRem(ctx, "surge:admitted", uid)
		pipe.HDel(ctx, "surge:admitted_expiry", uid)
		pipe.HDel(ctx, "surge:heartbeats", uid)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("cleanupExpiredAdmitted: %v", err)
	}
	log.Printf("freed %d expired admitted slots", len(expired))
}

func (m *Manager) cleanupStaleWaiting(ctx context.Context) {
	// Remove entries whose heartbeat hasn't been seen in WAITING_TTL.
	cutoff := float64(time.Now().Add(-time.Duration(m.cfg.WaitingTTL) * time.Second).UnixMilli())
	removed, err := m.rdb.ZRemRangeByScore(ctx, m.cfg.QueueName, "-inf", fmt.Sprintf("%f", cutoff)).Result()
	if err != nil {
		return
	}
	if removed > 0 {
		metrics.AbandonedTotal.Add(float64(removed))
		total, _ := m.rdb.ZCard(ctx, m.cfg.QueueName).Result()
		metrics.QueueDepth.Set(float64(total))
		log.Printf("removed %d stale waiting entries", removed)
	}
}
