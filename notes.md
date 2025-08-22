### closing channel when a wg is involved
NOTE: When you have multiple goroutines all doing the same task and are controlled by a wait group,
you can't close the chan that they use, inside the for loop, because their work is not done after the loop finishes(we're using
a wait group to track when their task finishes). So whenever there's a wg involved with a channel, you have to close it inside another goroutine.

Therefore, a pattern for closing the chan that they use, is to close it inside a gorutine after the wg.Wait():

```go
package main

import "sync"

var wg sync.WaitGroup

func main() {
	tasksCh := make(chan string)
	
	for i := 0; i < 10; i++ {
		wg.Add(1)
		
		go func() {
			defer wg.Done()
			
			// use the channel ...
        }()
		
		// ERROR: we can't close the channel here, because we should only close it after wg.Wait()
	}
	
	// instead, close it inside another goroutine after wg.Wait()
	go func() {
		wg.Wait()
		close(tasksCh)
    }()
}
```