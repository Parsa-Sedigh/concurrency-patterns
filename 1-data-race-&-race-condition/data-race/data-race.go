package main

import (
	"fmt"
	"sync"
)

var counter int

func increment(wg *sync.WaitGroup) {
	// Multiple goroutines read/write counter at the same time -> data race
	counter++
	wg.Done()
}

func main() {
	var wg sync.WaitGroup

	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go increment(&wg)

		/* If you call wg.Done() here, this will immediately mark the goroutine work as done which is wrong!
		The wg.Done() should be called when goroutine work is done, but here, we just spawned the goroutine. So pass the &wg to
		goroutine and call it from there.*/
	}

	wg.Wait()

	// Likely less than 1000 due to the clash
	fmt.Println(counter)
}
