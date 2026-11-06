package httpapi

import (
	"math"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// limiterSet keeps one token bucket per key (IP, email, or principal).
// Idle buckets are dropped so memory stays bounded.
type limiterSet struct {
	mu      sync.Mutex
	limit   rate.Limit
	burst   int
	buckets map[string]*bucket
	now     func() time.Time
	sweep   time.Time
}

type bucket struct {
	l    *rate.Limiter
	seen time.Time
}

func newLimiterSet(limit rate.Limit, burst int) *limiterSet {
	return &limiterSet{limit: limit, burst: burst, buckets: map[string]*bucket{}, now: time.Now}
}

// allow takes one token for key and, when none is left, returns how long
// to wait.
func (s *limiterSet) allow(key string) (bool, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if now.Sub(s.sweep) > time.Minute {
		for k, b := range s.buckets {
			// A bucket idle long enough to refill completely carries no state.
			if now.Sub(b.seen) > time.Duration(float64(s.burst)/float64(s.limit)*float64(time.Second))+time.Minute {
				delete(s.buckets, k)
			}
		}
		s.sweep = now
	}
	b, ok := s.buckets[key]
	if !ok {
		b = &bucket{l: rate.NewLimiter(s.limit, s.burst)}
		s.buckets[key] = b
	}
	b.seen = now
	r := b.l.ReserveN(now, 1)
	if delay := r.DelayFrom(now); delay > 0 {
		r.CancelAt(now)
		return false, delay
	}
	return true, 0
}

// limits groups the limiters the API uses.
type limits struct {
	loginIP    *limiterSet // 5 per minute per IP
	loginEmail *limiterSet // 10 per 15 minutes per email
	api        *limiterSet // 20/s per principal, burst 40
}

func newLimits() *limits {
	return &limits{
		loginIP:    newLimiterSet(rate.Every(time.Minute/5), 5),
		loginEmail: newLimiterSet(rate.Every(15*time.Minute/10), 10),
		api:        newLimiterSet(20, 40),
	}
}

func retryAfter(d time.Duration) string {
	return itoa(int(math.Ceil(d.Seconds())))
}

// clientIP returns the address of the client. X-Forwarded-For is honoured
// only when the direct peer is a trusted proxy; the rightmost untrusted hop
// is the client.
func (h *Handler) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	peer = peer.Unmap()
	if !h.trusted(peer) {
		return peer.String()
	}
	hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		a = a.Unmap()
		if !h.trusted(a) {
			return a.String()
		}
	}
	return peer.String()
}

func (h *Handler) trusted(a netip.Addr) bool {
	for _, p := range h.cfg.TrustedProxies {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
