package health

import (
	"net"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Status represents the health state of a connection.
type Status int

const (
	Unknown Status = iota
	Online
	Offline
	Degraded
)

func (s Status) String() string {
	switch s {
	case Online:
		return "online"
	case Offline:
		return "offline"
	case Degraded:
		return "degraded"
	default:
		return "unknown"
	}
}

// Result holds the health check outcome for a single connection.
type Result struct {
	ID      string
	Status  Status
	Latency time.Duration
}

// ResultMsg is a bubbletea message carrying health check results.
type ResultMsg struct {
	Results []Result
}

// TickMsg triggers a new round of health checks.
type TickMsg struct{}

const checkTimeout = 3 * time.Second

// Options configures the behaviour of a Checker.
type Options struct {
	Workers int           // max concurrent TCP dials (default 50)
	Timeout time.Duration // per-dial timeout (default 3s)
	Enabled bool          // false = skip all checks
}

// DefaultOptions returns sensible defaults for production use.
func DefaultOptions() Options {
	return Options{
		Workers: 50,
		Timeout: 3 * time.Second,
		Enabled: true,
	}
}

// breaker implements a per-host circuit breaker.
type breaker struct {
	failures  int       // consecutive failures
	openUntil time.Time // skip checks until this time
}

// breakerThreshold is the number of consecutive failures before the circuit opens.
const breakerThreshold = 3

// breakerBaseInterval is the base interval used for exponential backoff calculation.
const breakerBaseInterval = 30 * time.Second

// breakerMaxMultiplier caps the exponential backoff at 8x the base interval.
const breakerMaxMultiplier = 8

// Checker performs health checks with concurrency limiting and circuit breaking.
type Checker struct {
	opts     Options
	mu       sync.Mutex
	breakers map[string]*breaker // per-host circuit breaker
}

// NewChecker creates a Checker with the given options.
func NewChecker(opts Options) *Checker {
	return &Checker{
		opts:     opts,
		breakers: make(map[string]*breaker),
	}
}

// SetEnabled enables or disables health checking at runtime.
func (c *Checker) SetEnabled(enabled bool) {
	c.mu.Lock()
	c.opts.Enabled = enabled
	c.mu.Unlock()
}

// job is an internal work item for the worker pool.
type job struct {
	id   string
	addr string
}

// check performs a single TCP dial using the checker's configured timeout.
func (c *Checker) check(id, addr string) Result {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, c.opts.Timeout)
	latency := time.Since(start)

	if err != nil {
		return Result{ID: id, Status: Offline, Latency: latency}
	}
	conn.Close()

	if latency > 2*time.Second {
		return Result{ID: id, Status: Degraded, Latency: latency}
	}
	return Result{ID: id, Status: Online, Latency: latency}
}

// isOpen reports whether the circuit breaker for a host is currently open.
func (c *Checker) isOpen(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	b, ok := c.breakers[id]
	if !ok {
		return false
	}
	if b.failures < breakerThreshold {
		return false
	}
	return time.Now().Before(b.openUntil)
}

// recordResult updates the circuit breaker state after a check completes.
func (c *Checker) recordResult(r Result) {
	c.mu.Lock()
	defer c.mu.Unlock()

	b, ok := c.breakers[r.ID]
	if !ok {
		b = &breaker{}
		c.breakers[r.ID] = b
	}

	if r.Status == Offline {
		b.failures++
		if b.failures >= breakerThreshold {
			// Exponential backoff: interval * 2^(failures-threshold), capped at 8x.
			exp := b.failures - breakerThreshold
			multiplier := 1 << exp // 2^exp
			if multiplier > breakerMaxMultiplier {
				multiplier = breakerMaxMultiplier
			}
			b.openUntil = time.Now().Add(breakerBaseInterval * time.Duration(multiplier))
		}
	} else {
		// Successful check resets the breaker.
		b.failures = 0
		b.openUntil = time.Time{}
	}
}

// CheckAll runs health checks for all given addresses using a bounded worker pool
// and circuit breaker logic. targets maps connection ID -> host:port.
func (c *Checker) CheckAll(targets map[string]string) tea.Cmd {
	return func() tea.Msg {
		if !c.opts.Enabled {
			return ResultMsg{}
		}

		n := len(targets)
		if n == 0 {
			return ResultMsg{}
		}

		jobs := make(chan job, n)
		results := make(chan Result, n)

		// Spin up the worker pool, capped at the number of targets so we
		// don't create idle goroutines when the target list is small.
		workers := c.opts.Workers
		if workers > n {
			workers = n
		}

		var wg sync.WaitGroup
		wg.Add(workers)
		for i := 0; i < workers; i++ {
			go func() {
				defer wg.Done()
				for j := range jobs {
					var r Result
					if c.isOpen(j.id) {
						// Circuit is open — return cached offline without dialing.
						r = Result{ID: j.id, Status: Offline}
					} else {
						r = c.check(j.id, j.addr)
					}
					c.recordResult(r)
					results <- r
				}
			}()
		}

		// Enqueue all targets.
		for id, addr := range targets {
			jobs <- job{id: id, addr: addr}
		}
		close(jobs)

		// Close the results channel once every worker has finished.
		go func() {
			wg.Wait()
			close(results)
		}()

		collected := make([]Result, 0, n)
		for r := range results {
			collected = append(collected, r)
		}

		return ResultMsg{Results: collected}
	}
}

// ---- Backward-compatible package-level API --------------------------------

// Check performs a TCP dial to host:port and returns the result.
// It uses the package-level default timeout and is retained for backward
// compatibility.
func Check(id, addr string) Result {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, checkTimeout)
	latency := time.Since(start)

	if err != nil {
		return Result{ID: id, Status: Offline, Latency: latency}
	}
	conn.Close()

	if latency > 2*time.Second {
		return Result{ID: id, Status: Degraded, Latency: latency}
	}
	return Result{ID: id, Status: Online, Latency: latency}
}

// defaultChecker is used by the package-level CheckAll for backward compat.
var defaultChecker = NewChecker(DefaultOptions())

// CheckAll runs health checks for all given addresses concurrently.
// targets maps connection ID -> host:port.
// This package-level function delegates to a default Checker instance.
func CheckAll(targets map[string]string) tea.Cmd {
	return defaultChecker.CheckAll(targets)
}

// ScheduleTick returns a tea.Cmd that fires a TickMsg after the interval.
func ScheduleTick(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return TickMsg{}
	})
}
