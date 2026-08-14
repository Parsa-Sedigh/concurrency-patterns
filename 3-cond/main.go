package main

import (
	"errors"
	"sync"
)

var (
	ErrClosed = errors.New("queue is closed")
)

// queue is bounded queue
type queue struct {
	mu       sync.Mutex
	notEmpty *sync.Cond // we wanna wait for when the queue is NOT empty. Map this requirement to fields exactly
	notFull  *sync.Cond

	items       []int
	maxItemsNum int
	closed      bool
}

func newQueue(maxItemsNum int) *queue {
	q := &queue{maxItemsNum: maxItemsNum}
	q.notEmpty = sync.NewCond(&q.mu)
	q.notFull = sync.NewCond(&q.mu)

	return q
}

func (q *queue) put(item int) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) == q.maxItemsNum && !q.closed {
		q.notFull.Wait()
	}
	if q.closed {
		return ErrClosed
	}

	q.items = append(q.items, item)
	q.notEmpty.Signal() // added exactly one item, so wake exactly one getter

	return nil
}

func (q *queue) get() (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) == 0 && !q.closed {
		q.notEmpty.Wait()
	}
	if q.closed {
		return 0, ErrClosed
	}

	v := q.items[0]
	q.items = q.items[1:]

	q.notFull.Signal() // freed exactly one slot, maybe some goroutine is waiting for this, so wake exactly one putter

	return v, nil
}

func (q *queue) close() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.closed = true

	// ALL goroutines should recheck and leave
	q.notEmpty.Broadcast()
	q.notFull.Broadcast()
}
