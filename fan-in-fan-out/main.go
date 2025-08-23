package main

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

type Task struct {
	ID int
}

type Result struct {
	Value int
	Err   error
}

func main() {
	rand.Intn(10)

	// Create sample tasks
	numTasks := 20
	tasks := make([]Task, 0, 10)
	for i := 0; i < numTasks; i++ {
		tasks = append(tasks, Task{
			ID: i,
		})
	}

	ctx := context.Background()
	//results := workerPoolWithStaticDivision(ctx, tasks, 4)
	//results := workerPoolWithDynamicDivision(ctx, tasks, 4)
	results := workerPoolWithRoundRobin(ctx, tasks, 4)

	fmt.Println("results:", results)
	fmt.Println("done")
}

func workerPoolWithStaticDivision(ctx context.Context, tasks []Task, numWorkers int) []Result {
	resCh := make(chan Result)

	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		// Task Division
		start, end := getTaskRange(TaskRangeReq{
			NumTasks:   len(tasks),
			NumWorkers: numWorkers,
			WorkerNum:  i,
		})

		tasksChunk := tasks[start:end]

		// Worker Assignment
		wg.Add(1)
		go func(tasks []Task) {
			defer wg.Done()

			for _, task := range tasksChunk {
				select {
				case <-ctx.Done():
					return

				default:
					res := processTask(task)

					select {
					case <-ctx.Done():
						return

					case resCh <- res:
					}
				}
			}
		}(tasksChunk)
	}

	go func() {
		wg.Wait()
		close(resCh)
	}()

	results := make([]Result, 0, len(tasks))
	for res := range resCh {
		results = append(results, res)
	}

	return results
}

func workerPoolWithDynamicDivision(ctx context.Context, tasks []Task, numWorkers int) []Result {
	var wg sync.WaitGroup

	// Buffered channel to reduce contention
	tasksCh := make(chan Task, len(tasks))

	// we're using fan-in:
	resultCh := make(chan Result, len(tasks))

	// Workers pull tasks from tasksCh as they complete previous tasks. The distribution is not strictly equal because workers pull tasks dynamically
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			select {
			case <-ctx.Done():
				return

			case task, ok := <-tasksCh:
				if !ok {
					// Channel is closed, exit
					return
				}

				res := processTask(task)

				select {
				case <-ctx.Done():
					return

				case resultCh <- res:
				}
			}
		}()
	}

	// distributer goroutine - sends tasks to workers
	go func() {
		for _, task := range tasks {
			select {
			case <-ctx.Done():
				return

			// then, the worker goroutine is gonna pick up a task from tasksCh
			case tasksCh <- task:
			}
		}

		// Close the tasks cha after all tasks are sent
		close(tasksCh)
	}()

	// Close results chan after all workers are done
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	results := make([]Result, 0, len(tasks))
	for res := range resultCh {
		results = append(results, res)
	}

	return results
}

type TaskRangeReq struct {
	NumTasks   int
	NumWorkers int
	WorkerNum  int
}

// remain: 3
// numWorker: 0 -> 0-49 -> 50
// numWorker: 1 -> 50-99 -> 51
func getTaskRange(req TaskRangeReq) (int, int) {
	numTasksPerWorker := req.NumTasks / req.NumWorkers
	numRemainingTasks := req.NumTasks % req.NumWorkers

	var (
		hadAdditionalWork bool
		curAdditionalWork int
		start             = req.WorkerNum * numTasksPerWorker
	)

	if numRemainingTasks > 0 && req.WorkerNum > 0 && req.WorkerNum-1 < numRemainingTasks {
		hadAdditionalWork = true
		start++
	}

	if req.WorkerNum < numRemainingTasks {
		curAdditionalWork++
	}

	end := start + numTasksPerWorker + curAdditionalWork

	fmt.Println("hadAdditionalWork", hadAdditionalWork, "start", start, "end", end)

	return start, end
}

func processTask(task Task) Result {
	fmt.Printf("Processing task %d...\n", task.ID)

	randVal := rand.Intn(1000)
	time.Sleep(time.Duration(randVal) * time.Millisecond)

	if randVal%2 == 0 {
		return Result{
			Value: 0,
			Err:   fmt.Errorf("task %d failed", task.ID),
		}
	}

	fmt.Println("processing done")

	return Result{Value: randVal}
}
