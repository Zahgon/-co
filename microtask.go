package co

import "sync"

// microQueue is an unbounded FIFO job queue drained by a single dedicated
// goroutine. It stands in for the JavaScript microtask queue.
//
// Draining serially on ONE goroutine is what makes this port faithful: in
// JavaScript every promise reaction runs to completion before the next one
// starts, so `co`'s trampoline never observes interleaving. Reproducing that
// here removes an entire class of ordering differences (and data races) that a
// "goroutine per callback" design would introduce.
//
// The queue must be unbounded: a running job may schedule further jobs, and a
// bounded channel would deadlock the drainer against itself once full.
type microQueue struct {
	mu    sync.Mutex
	cond  *sync.Cond
	items []func()
}

var (
	queue     *microQueue
	queueOnce sync.Once
)

func newMicroQueue() *microQueue {
	q := &microQueue{}
	q.cond = sync.NewCond(&q.mu)
	return q
}

// schedule enqueues fn to run on the microtask goroutine.
//
// Callers must never block the microtask goroutine waiting on work that can
// only be completed by another microtask; see the package documentation for
// the (short) list of rules.
func schedule(fn func()) {
	queueOnce.Do(func() {
		queue = newMicroQueue()
		go queue.run()
	})
	queue.mu.Lock()
	queue.items = append(queue.items, fn)
	queue.mu.Unlock()
	queue.cond.Signal()
}

func (q *microQueue) run() {
	microtaskGoroutine.Store(goid())

	for {
		q.mu.Lock()
		for len(q.items) == 0 {
			q.cond.Wait()
		}
		fn := q.items[0]
		q.items[0] = nil
		q.items = q.items[1:]
		q.mu.Unlock()

		q.invoke(fn)
	}
}

// invoke runs fn, swallowing panics so a single misbehaving reaction cannot
// take down the scheduler for the whole process. Every reaction registered by
// this package already converts panics into rejections before reaching here,
// so this is strictly a backstop.
func (q *microQueue) invoke(fn func()) {
	defer func() {
		_ = recover()
	}()
	fn()
}
