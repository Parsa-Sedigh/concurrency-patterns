package main

import (
	"fmt"
	"sync"
	"time"
)

/* Here, a mutex prevents a data race, but there’s still a race condition in the line where we wanna see the val of counter before
waiting for all goroutines to finish using wg.Wait() . The value of "Counter before wait" depends on
how many goroutines finish before the time.Sleep ends. It changes every run because of timing.*/

var counter int
var mu sync.Mutex

func increment(wg *sync.WaitGroup) {
	mu.Lock()

	// Protected by a mutex -> no data race
	counter++
	mu.Unlock()
	wg.Done()
}

func main() {
	var wg sync.WaitGroup

	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go increment(&wg)
	}

	// Wait a tiny bit
	time.Sleep(time.Millisecond)

	fmt.Println("Counter before wait:", counter) // Could be 50, 200, etc.
	fmt.Println(counter)

	wg.Wait()

	fmt.Println(counter)
}
