package health

import (
	"net"
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

// Check performs a TCP dial to host:port and returns the result.
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

// CheckAll runs health checks for all given addresses concurrently.
// targets maps connection ID -> host:port.
func CheckAll(targets map[string]string) tea.Cmd {
	return func() tea.Msg {
		results := make([]Result, 0, len(targets))
		ch := make(chan Result, len(targets))

		for id, addr := range targets {
			go func(id, addr string) {
				ch <- Check(id, addr)
			}(id, addr)
		}

		for range targets {
			results = append(results, <-ch)
		}

		return ResultMsg{Results: results}
	}
}

// ScheduleTick returns a tea.Cmd that fires a TickMsg after the interval.
func ScheduleTick(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return TickMsg{}
	})
}
