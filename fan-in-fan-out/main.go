package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"
)

/*
	FAN-OUT / FAN-IN, pipeline style. This is the shape to reach for in an interview.

	                    ┌──► worker ──► out1 ──┐
	   gen ──► tasks ───┼──► worker ──► out2 ──┼──► merge ──► out ──► collect
	                    └──► worker ──► out3 ──┘

	   FAN-OUT: N goroutines all reading from the SAME input channel. Whoever is free takes the next task,
	            so the work balances itself. numWorkers is the concurrency limit.

	   FAN-IN:  N output channels combined into ONE. That is what merge() does.

	Three rules make the whole thing work. Say them out loud in an interview:

	 1. Every stage creates its own output channel, returns it immediately, and closes it when done.
	    "The sender closes" - a receiver must never close, and nobody closes a channel they did not create.

	 2. Every SEND is inside a select{} with ctx.Done(). Every RECEIVE is a plain `for range`.
	    They are not symmetric: a blocked receive is woken by the upstream close, but a blocked send is only
	    woken by a receiver (closing under a blocked sender panics). So only sends need an escape route.

	 3. A stage does its work in a goroutine and returns the channel straight away, so stages run at the
	    same time instead of one after another.

	Follow those and cancellation and shutdown fall out for free: cancel ctx -> gen stops sending and closes
	its output -> workers' `for range` loops end and they close theirs -> merge's copiers finish, wg hits
	zero, merge closes out -> the collect loop below ends. Every goroutine exits, nothing leaks.
*/

type Task struct {
	ID int
}

type Result struct {
	Value int
	Err   error
}

// ---------------------------------------------------------------- the pipeline

// gen is the SOURCE stage: it turns a slice into a channel so the rest of the pipeline has something to read.
func gen(ctx context.Context, tasks []Task) <-chan Task {
	out := make(chan Task)

	go func() {
		defer close(out) // rule 1: this stage owns `out`, so this stage closes it, on every exit path(by using defer)

		for _, task := range tasks {
			select {
			case <-ctx.Done(): // rule 2: a send always needs an escape route
				return

			case out <- task:
			}
		}
	}()

	return out // rule 3: return now, work happens in the background
}

// worker is ONE fan-out branch. It reads from the shared `in` channel and returns its OWN output channel.
//
// Calling this N times with the same `in` is the fan-out. There is no task splitting and no round-robin:
// every worker competes for the same channel, so a worker that finishes early just takes the next task.
// That is why this balances uneven task durations by itself.
func worker(ctx context.Context, in <-chan Task) <-chan Result {
	out := make(chan Result)

	go func() {
		defer close(out)

		/* `for range` and not `for { select { <-ctx.Done() ... } }`: this loop already ends when gen() closes
		`in`, and gen closes `in` as soon as ctx is cancelled. Checking ctx here too would be a second door
		onto the same event. See ../worker-pool/receiving-tasks.md.*/
		for task := range in {
			res := processTask(ctx, task)

			select {
			case <-ctx.Done():
				return

			case out <- res:
			}
		}
	}()

	return out
}

// merge is the FAN-IN: many channels in, one channel out.
//
// One copier goroutine per input channel, all sending to the same `out`. The WaitGroup counts the copiers,
// and a separate goroutine closes `out` once they are all done.
//
// Q: Why does the close need its own goroutine?
// A: Because wg.Wait() blocks. Calling it here would block merge itself, so merge would never return `out`,
// so nobody downstream could ever receive, so the copiers could never finish. Deadlock. This is because if we call wg.Wait()
// and close(out) without a goroutine, merge() will wait until it's out chan is drained, but no goroutine is actually consuming
// that out chan since we haven't returned from merge() yet! So we have to return ASAP so the consumers can begin consuming the
// out chan. How to return ASAP? By spawning a goroutine for wg.Wait() and close(out).
// Same reason the collector goroutine exists in ../worker-pool/main.go.
//
// Q: Why a WaitGroup instead of closing after the last one?
// A: There is no "last one" to detect. The copiers finish in any order, and `out` must stay open until every
// input channel is drained. Counting is the only way to know.
func merge(ctx context.Context, cs ...<-chan Result) <-chan Result {
	out := make(chan Result)

	var wg sync.WaitGroup
	wg.Add(len(cs)) // add up front, not inside the goroutines, or Wait can see zero and return early

	for _, c := range cs {
		// Go 1.22+ gives each iteration its own `c`. Before 1.22 you had to pass it in: go func(c <-chan Result){...}(c)
		go func() {
			defer wg.Done()

			for res := range c {
				select {
				case <-ctx.Done():
					return

				case out <- res:
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}

// ---------------------------------------------------------------- putting it together

// fanOutFanIn is the whole pattern in eight lines. This is the part worth memorising.
func fanOutFanIn(ctx context.Context, tasks []Task, numWorkers int) ([]Result, error) {
	in := gen(ctx, tasks)

	// FAN-OUT: numWorkers branches, all reading the same `in`.
	branches := make([]<-chan Result, 0, numWorkers)
	for i := 0; i < numWorkers; i++ {
		branches = append(branches, worker(ctx, in))
	}

	// FAN-IN: collapse them back into one channel.
	out := merge(ctx, branches...)

	results := make([]Result, 0, len(tasks))
	for res := range out {
		results = append(results, res)
	}

	/* Results arrive in completion order, never task order. If the caller needs results[i] to match tasks[i],
	put the index in Task and Result and sort at the end, or drop channels and write into a pre-sized slice.

	ctx.Err() is nil unless we were cancelled, so this one line covers both cases. Without it, a cancelled run
	looks exactly like a short task list: fewer results and no explanation.*/
	return results, ctx.Err()
}

// ---------------------------------------------------------------- the work itself

func processTask(ctx context.Context, task Task) Result {
	select {
	case <-ctx.Done():
		return Result{Err: ctx.Err()}

	case <-time.After(time.Duration(rand.Intn(200)) * time.Millisecond):
	}

	if task.ID%5 == 0 {
		return Result{Err: fmt.Errorf("task %d failed", task.ID)}
	}

	return Result{Value: task.ID * 10}
}

func main() {
	tasks := make([]Task, 0, 20)
	for i := range 20 {
		tasks = append(tasks, Task{ID: i})
	}

	// normal run: every task comes back
	results, err := fanOutFanIn(context.Background(), tasks, 4)
	log.Printf("normal:    %d results, err=%v", len(results), err)

	// cancelled run: partial results, and the error says why
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	results, err = fanOutFanIn(ctx, tasks, 4)
	log.Printf("cancelled: %d results, err=%v", len(results), err)
}

/*
	INTERVIEW NOTES

	When is merge() actually needed?
	  Only when the producers already have separate channels. If you are writing the workers yourself, having
	  them all send to one shared result channel is simpler and does the same job - that is what
	  ../worker-pool/main.go does. Reach for merge when the channels come from somewhere you do not control:
	  several upstream services, several pipeline branches, one channel per shard.

	Common follow-up questions:

	  "Stop everything on the first error."
	    Use errgroup.WithContext. The first non-nil error cancels the shared ctx, which stops gen, which
	    closes the pipeline from the top down.

	  "Limit concurrency."
	    numWorkers already does it. Nothing else is needed - the workers share one input channel, so at most
	    numWorkers tasks are in flight.

	  "Keep the results in order."
	    Channels cannot do it. Carry an index on Task and Result and sort at the end, or write into a
	    pre-sized slice by index and skip channels entirely.

	  "What if a worker panics?"
	    defer close(out) still runs, so the pipeline drains instead of hanging - but the process dies anyway.
	    recover() inside the worker goroutine, turn it into a Result{Err: ...}, and carry on.

	  "Why is the input channel unbuffered?"
	    It does not need a buffer: workers are already waiting on it. A buffer would only let gen run ahead,
	    and on cancellation the workers would still drain everything sitting in it before stopping.

	Mistakes that cost people the question:
	  - Closing a channel from the receiving side, or closing one you did not create.
	  - Calling wg.Wait() inline instead of in its own goroutine, so out never closes. Deadlock.
	  - wg.Add() inside the goroutine instead of before starting it, so Wait can see zero and return early.
	  - Sending without a select on ctx.Done(), so a cancelled pipeline leaks a blocked sender forever.
	  - Forgetting that fan-in output is unordered.
*/
