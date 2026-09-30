package admission

import (
	"context"
	"errors"
	"sync"
	"time"
)

type Priority string

const (
	Interactive Priority = "interactive"
	Batch       Priority = "batch"
)

var ErrShed = errors.New("request shed by admission controller")

type Config struct {
	MaxRunning         int `json:"max_running"`
	InteractiveReserve int `json:"interactive_reserve"`
	MaxQueuedTokens    int `json:"max_queued_tokens"`
}

func (c *Controller) Config() Config {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.config
}

type Snapshot struct {
	Running          int `json:"running"`
	QueuedRequests   int `json:"queued_requests"`
	QueuedTokens     int `json:"queued_tokens"`
	InteractiveQueue int `json:"interactive_queue"`
	BatchQueue       int `json:"batch_queue"`
}

type waiter struct {
	priority Priority
	tokens   int
	ready    chan struct{}
	granted  bool
}

type Controller struct {
	mu      sync.Mutex
	config  Config
	running int
	queued  int
	waiters []*waiter
}

type Lease struct {
	controller *Controller
	once       sync.Once
}

func New(config Config) *Controller {
	if config.MaxRunning < 1 {
		config.MaxRunning = 1
	}
	if config.InteractiveReserve < 0 {
		config.InteractiveReserve = 0
	}
	if config.InteractiveReserve >= config.MaxRunning {
		config.InteractiveReserve = config.MaxRunning - 1
	}
	if config.MaxQueuedTokens < 1 {
		config.MaxQueuedTokens = 1
	}
	return &Controller{config: config}
}

func (c *Controller) Acquire(ctx context.Context, priority Priority, tokens int) (*Lease, time.Duration, error) {
	if priority != Batch {
		priority = Interactive
	}
	if tokens < 1 {
		tokens = 1
	}
	started := time.Now()

	c.mu.Lock()
	if c.canRun(priority) {
		c.running++
		c.mu.Unlock()
		return &Lease{controller: c}, 0, nil
	}
	// Batch work is deliberately lossy under pressure. It never occupies the
	// queue needed by deadline-bound interactive traffic; callers receive an
	// explicit retryable response instead of turning overload into a timeout.
	if priority == Batch {
		c.mu.Unlock()
		return nil, 0, ErrShed
	}
	if tokens > c.config.MaxQueuedTokens || c.queued+tokens > c.config.MaxQueuedTokens {
		c.mu.Unlock()
		return nil, 0, ErrShed
	}

	w := &waiter{priority: priority, tokens: tokens, ready: make(chan struct{})}
	c.waiters = append(c.waiters, w)
	c.queued += tokens
	c.mu.Unlock()

	select {
	case <-w.ready:
		return &Lease{controller: c}, time.Since(started), nil
	case <-ctx.Done():
		c.mu.Lock()
		if w.granted {
			c.mu.Unlock()
			return &Lease{controller: c}, time.Since(started), nil
		}
		for index, candidate := range c.waiters {
			if candidate == w {
				c.waiters = append(c.waiters[:index], c.waiters[index+1:]...)
				c.queued -= w.tokens
				break
			}
		}
		c.mu.Unlock()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, time.Since(started), ErrShed
		}
		return nil, time.Since(started), ctx.Err()
	}
}

func (c *Controller) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	snapshot := Snapshot{Running: c.running, QueuedRequests: len(c.waiters), QueuedTokens: c.queued}
	for _, candidate := range c.waiters {
		if candidate.priority == Interactive {
			snapshot.InteractiveQueue++
		} else {
			snapshot.BatchQueue++
		}
	}
	return snapshot
}

func (l *Lease) Release() {
	if l == nil || l.controller == nil {
		return
	}
	l.once.Do(l.controller.release)
}

func (c *Controller) release() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running > 0 {
		c.running--
	}
	c.dispatch()
}

func (c *Controller) canRun(priority Priority) bool {
	if priority == Interactive {
		return c.running < c.config.MaxRunning
	}
	batchLimit := c.config.MaxRunning - c.config.InteractiveReserve
	return c.running < batchLimit
}

func (c *Controller) dispatch() {
	for {
		index := c.nextRunnable()
		if index < 0 {
			return
		}
		w := c.waiters[index]
		c.waiters = append(c.waiters[:index], c.waiters[index+1:]...)
		c.queued -= w.tokens
		c.running++
		w.granted = true
		close(w.ready)
	}
}

func (c *Controller) nextRunnable() int {
	if c.running >= c.config.MaxRunning {
		return -1
	}
	for index, candidate := range c.waiters {
		if candidate.priority == Interactive {
			return index
		}
	}
	if !c.canRun(Batch) {
		return -1
	}
	for index, candidate := range c.waiters {
		if candidate.priority == Batch {
			return index
		}
	}
	return -1
}
