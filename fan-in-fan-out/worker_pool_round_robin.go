package main

import (
	"context"
	"sync"
)

func workerPoolWithRoundRobin(ctx context.Context, tasks []Task, numWorkers int) []Result {
	if numWorkers <= 0 {
		numWorkers = 1
	}

	/* Q: When we wanna make sure that we close the workerChs after all workers actually finished their work,
	 	  why can't we use a single waitgroup when we want to wait for ? Why do we need two discrete wait groups?
	   A: Because if we just use a single wg, it's only gonna get decremented after a worker goroutine returns. But the worker goroutine func
		  will never return(finish) since it's blocked on the select{} of `<- workerCh` and the goroutine that is responsible for
		  closing the `workerCh` is waiting on the worker goroutines! So two goroutines are waiting for each other, deadlock.
	      In other words:
	      This creates a circular wait: workers block on receiving tasks(<-workerCh) so they won't call defer wg.Done() unless workerCh is closed,
		  but the distributor won't close the worker chans because it's blocked on workers finishing(wg.Wait()).*/
	var (
		taskWg   sync.WaitGroup // for waiting tasks getting processed
		resultWg sync.WaitGroup // for waiting whole workers finished
	)

	// Create the slice for worker-specific channels
	workerChs := make([]chan Task, numWorkers)
	resultCh := make(chan Result, len(tasks))

	// initialize each chan in workerChs(if we don't do this, we're gonna get deadlock because the channels would be nil)
	for i := range workerChs {
		workerChs[i] = make(chan Task, len(tasks)/numWorkers)
	}

	for i := 0; i < numWorkers; i++ {
		resultWg.Add(1)

		go func(workerCh <-chan Task) {
			defer resultWg.Done()

			for {
				select {
				case <-ctx.Done():
					return

				case task, ok := <-workerCh:
					if !ok {
						return
					}

					taskWg.Add(1)
					res := processTask(task)
					taskWg.Done()

					select {
					case <-ctx.Done():
						return

					case resultCh <- res:
					}
				}
			}
		}(workerChs[i])
	}

	// distributer goroutine Distribute tasks in round-robin fashion
	go func() {
		for i, task := range tasks {
			select {
			case <-ctx.Done():
				return

			case workerChs[i%numWorkers] <- task:
			}
		}

		taskWg.Wait()

		for _, workerCh := range workerChs {
			close(workerCh)
		}
	}()

	// Close resultCh when all workers are done
	go func() {
		resultWg.Wait()
		close(resultCh)
	}()

	results := make([]Result, 0, len(tasks))
	for res := range resultCh {
		results = append(results, res)
	}

	return results
}
