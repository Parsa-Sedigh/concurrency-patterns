package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

func main() {}

type Task struct{}

type Result struct {
	Value int
	Err   error
}

func workerPoolWithContext(ctx context.Context, tasks []Task, numWorkers int) []Result {
	// 1. create channels
	tasksCh := make(chan Task)
	resultCh := make(chan Result)

	var wg sync.WaitGroup

	// 2. start workers(goroutines) - each task is done by one goroutine
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)

		go func(id int) {
			fmt.Println(i, id)

			select {
			case task, ok := <-tasksCh:
				if !ok {
					// Channel is closed, exit worker
					return
				}

				result := processTask(task)

				select {
				// Check cancellation before sending result
				case <-ctx.Done():
					return

				// Send result
				case resultCh <- result:
				}
			}
		}(i)
	}

	// 3. send tasks to workers. Here, we do task distribution which is done via a channel,
	go func() {
		for _, task := range tasks {
			/* Q: Why do we need to spawn a goroutine here? */
			/* A:
			Non-blocking task sending: Sending tasks to taskCh can block if the channel’s buffer is full or if workers are busy.
			Without a goroutine, the main goroutine would block on taskCh <- task, halting the program until a worker receives the task.
			A separate goroutine allows task distribution to proceed independently, keeping the main flow free to
			handle other logic (e.g., starting workers or collecting results).

			So Distributing tasks concurrently allows the program to scale, as the task sender doesn’t wait for
			each task to be processed before sending the next.*/

			/* We can't just do: taskCh <- task because we need to simultaneously check if ctx is cancelled everytime we wanna
			send a task to the task channel. Now to check for this, we need a select{}*/
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
	/* A: A separate goroutine handles the waiting, allowing the main goroutine to focus on reading from resultCh immediately.
	If the main goroutine called wg.Wait() directly, it couldn’t move to collecting results concurrently, as it would be stuck waiting.
	By spawning a goroutine for this task which is another blocking task, maximizes efficiency.
	The main goroutine can start gathering results immediately, rather than waiting for all tasks to complete before starting collection.*/
	results := make([]Result, 0, len(tasks))
	go func() {
		// Wait for all workers to finish
		wg.Wait()
		close(resultCh)
	}()

	for res := range resultCh {
		results = append(results, res)
	}

	// 5. return results
	return results
}

func processTask(task Task) Result {
	fmt.Println("processing task ...")

	randVal := rand.Intn(1000)
	time.Sleep(time.Duration(randVal) * time.Millisecond)

	if randVal%2 == 0 {
		return Result{Value: randVal, Err: errors.New("some err msg")}
	}

	return Result{
		Value: randVal,
	}
}
