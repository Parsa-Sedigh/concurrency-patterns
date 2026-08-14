package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"
)

type Task struct {
	name string
}

type Result struct {
	Value int
	Err   error
}

func main() {
	results, err := workerPoolWithContext(context.Background(), []Task{{name: "task 1"}, {name: "task 2"}, {name: "task 3"}}, 3)
	if err != nil {
		/* Without this, a cancelled run is indistinguishable from a short task list: you just get fewer
		results back and no way to tell why.*/
		log.Println("pool stopped early:", err)
	}

	log.Println(results)
}

func workerPoolWithContext(ctx context.Context, tasks []Task, numWorkers int) ([]Result, error) {
	// 1. create channels
	tasksCh := make(chan Task)
	resultCh := make(chan Result)

	var wg sync.WaitGroup

	// 2. start workers(goroutines) - each task is done by one goroutine
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			/* On which loop to use - `for task := range tasksCh` vs `for { select { ... } }` -
			see receiving-tasks.md in this folder. Short version: with one sender that does
			`defer close(tasksCh)`, the close already wakes every idle worker, so watching
			ctx.Done() here as well buys nothing. Use `for range`. */
			for task := range tasksCh {
				/* Note: Checking for ctx.Done() inside a select{} here buys nothing. Because we're using for range chan which
				receives cancellation from sender. The sender is watching ctx.Done() itself.*/
				result := processTask(ctx, task)

				/*  Why another select{} here?

				If you wanna go extreme and throw away the work that was done after ctx is done, you would want to do another
				select{} here before sending the result into it's channel. Otherwise(which is what we want most of the time),
				don't check ctx.Done() again here, just send the res to chan.*/
				select {
				case <-ctx.Done():
					return

				case resultCh <- result:
				}
			}
		}()
	}

	// 3. send tasks to workers. Here, we do task distribution which is done via a channel,
	go func() {
		/* The sender has to close tasksCh on every exit path, including the cancelled one. One way is to close it before
		returning in <- ctx.Done() case, but to be sure that it's closed when the goroutine returns, we do it inside a defer.

		This close is also what stops the workers. Closing a chan wakes every goroutine blocked receiving on it at once,
		so cancelling ctx here reaches the idle workers without them watching ctx themselves. See receiving-tasks.md.*/
		defer close(tasksCh)

		for _, task := range tasks {
			/* Q: Why do we need to spawn a goroutine for this sending loop? */
			/* A: Not for speed. For correctness - inline, this deadlocks.

			Say 5 tasks and 3 workers. The first 3 sends succeed. The 4th send blocks, because every worker is busy.
			So we never reach the `for res := range resultCh` loop at the bottom of this func. The workers then finish
			their tasks and block on `resultCh <- result` with nobody reading. Everything stops.

			Sending in its own goroutine is what lets the sending and the collecting happen at the same time.
			(It only works inline while len(tasks) <= numWorkers, which is why it looks fine in small examples.)*/

			/* We can't just do: taskCh <- task because we need to simultaneously check if ctx is cancelled everytime we wanna
			send a task to the task channel.

			REMEMBER: receiving and sending are not symmetric.

			Always pair a SEND with select{ case <- ctx.Done() ...} . This will also make consumers to avoid having to check for
			ctx.Done() themselves because it's being done by the sender. The consumer can do this when it's using `for range chan`
			which works this way:

			- If chan is unbuffered, it immediately wakes an idle consumer.
			- If chan is buffered, the worker consumes all previously accumulated msgs in chan before getting to the close signal.

			If you want the consumer to leave the prev accumulated work, use select{ case <- ctx.Done() } in the consumer in addition
			to the sender. */

			select {
			// Stop sending tasks if ctx is canceled
			case <-ctx.Done():
				return

			// Send task to workers
			case tasksCh <- task:
			}
		}
	}()

	// 4. collect results. Here, we do result collection which is done via a channel,
	/* Q: Why do we need to spawn a goroutine here? */
	/* A: Same answer as the sender above - correctness, not speed. Inline, this deadlocks every time.

	resultCh is unbuffered, so a worker's send blocks until someone receives. Calling wg.Wait() here directly would
	block before the `for res := range resultCh` loop below ever started, so the workers could never finish, so Wait
	would never return.

	Running Wait in its own goroutine lets it happen at the same time as that loop. And close(resultCh) after Wait is
	what ends the loop - without the close, the range would block forever once the last result was read.*/
	results := make([]Result, 0, len(tasks))
	go func() {
		// Wait for all workers to finish
		wg.Wait()
		close(resultCh)
	}()

	/* Results arrive in completion order, not task order. If you ever need results[i] to line up with tasks[i],
	collect into a pre-sized slice by index instead of a channel.*/
	for res := range resultCh {
		results = append(results, res)
	}

	/* 5. return results. ctx.Err() is nil when we were not cancelled, so this one line covers both cases.
	On the cancelled path the results are partial - the tasks that never got sent are simply missing from the slice,
	which is why the caller needs the error to tell "cancelled" apart from "short task list".*/
	return results, ctx.Err()
}

func processTask(ctx context.Context, task Task) Result {
	fmt.Println("processing", task.name, "...")

	select {
	case <-ctx.Done():
		return Result{Value: -1, Err: fmt.Errorf("context canceled: %w", ctx.Err())}

	case <-time.After(time.Second):
		randVal := rand.Intn(1000)

		if randVal%2 == 0 {
			log.Printf("processed task %v with error \n", task.name)

			return Result{Value: randVal, Err: errors.New("some err msg")}
		}

		log.Printf("processed task %v\n", task.name)

		return Result{
			Value: randVal,
		}
	}
}
