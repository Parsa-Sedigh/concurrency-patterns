package main

import (
	"fmt"
	"math"
	"os"
	"sync"
	"text/tabwriter"
	"time"
)

func producer(wg *sync.WaitGroup, l sync.Locker) {
	defer wg.Done()

	for i := 0; i < 5; i++ {
		l.Lock()
		l.Unlock()

		time.Sleep(1)
	}
}

func observer(wg *sync.WaitGroup, l sync.Locker) {
	defer wg.Done()

	l.Lock()
	l.Unlock()
}

// test runs a test with one producer and count number of observers, measuring how long it takes to complete.
// Measures how lock contention affects performance.
// Tests mutex (full lock for all) vs. rwMutex (read lock for observers, write(full) lock for producer).
// Params:
// - count: Number of observer goroutines. So overall, we have count + 1 goroutines to wait for(`count` observers(readers) and 1 producer(writer))
// - mutex sync.Locker: Lock for the producer or writer. Usually we're given a write lock or full mutex).
// - rwMutex sync.Locker: Lock for observers or readers. Usually we're given a read-lock(rwMutex) but could be z mutex too(which means a full mutex).
/* What this func does?
- Creates a WaitGroup and adds count+1 (for count observers + 1 producer).
- Records the start time (time.Now()).
- Launches one producer goroutine with a Mutex(full mutex - writes could use of this).
- Launches count observer goroutines with rwMutex.
- Waits for all goroutines to finish (wg.Wait()).
- Returns the elapsed time (time.Since).
*/
func test(count int, mutex, rwMutex sync.Locker) time.Duration {
	var wg sync.WaitGroup

	wg.Add(count + 1)

	begin := time.Now()

	go producer(&wg, mutex)

	for i := 0; i < count; i++ {
		go observer(&wg, rwMutex)
	}

	wg.Wait()

	return time.Since(begin)
}

func main() {
	// Create a tabwriter to format output as a table.
	tw := tabwriter.NewWriter(os.Stdout, 0, 1, 2, ' ', 0)
	defer tw.Flush()

	var m sync.RWMutex

	// Print a header: “Num of Readers” (number of observers), “Mutex” (time with full locks), “RWMutex” (time with read locks).
	fmt.Fprintf(tw, "Num of readers\tMutex\tRWMutex\n")

	// Loops 20 times, with count as powers of 2 (1, 2, 4, ..., 524288).
	for i := 0; i < 20; i++ {
		count := int(math.Pow(2, float64(i)))

		// Print the number of observers involved in the test and durations for both tests.
		fmt.Fprintf(tw, "%d\t%v\t%v\n", count,
			test(count, &m, &m),          // producer uses full mutex, observer uses full mutex
			test(count, &m, m.RLocker())) // producer uses full mutex, observer uses read mutex
	}
}
