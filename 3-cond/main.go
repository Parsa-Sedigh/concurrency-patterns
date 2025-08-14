package main

import (
	"math/rand"
	"sync"
	"time"
)

var (
	pokemonList = []string{"Pikachu", "Charmander", "Squirtle", "Bulbasaur", "Jigglypuff"}

	// shard state that is mutated
	pokemon = ""
	mu      = &sync.Mutex{}
	cond    = sync.NewCond(mu)
)

func main() {
	var wg sync.WaitGroup

	wg.Add(2)

	// Consumer(waiter goroutine)
	go func() {
		defer wg.Done()

		cond.L.Lock()
		defer cond.L.Unlock()

		/* When woken, rechecks the condition (loop ensures this).

		Q: Why use a for loop when checking the condition?
		A:
		*/
		for pokemon != "Pikachu" {
			cond.Wait()
		}

		// Now Condition is definitely true, since this goroutine has exclusive access to the mutex of cond

		println("Caught" + pokemon)
		pokemon = ""
	}()

	// producer(signaler goroutine)
	go func() {
		defer wg.Done()

		// Every 1ms, a random Pokémon appears

		time.Sleep(1 * time.Millisecond)

		for i := 0; i < 100; i++ {
			cond.L.Lock()
			pokemon = pokemonList[rand.Intn(len(pokemonList))] // Update shared state

			/* Note: Don't call Signal() after unlocking the mutex. It will create a small window where another goroutine could
			modify pokemon after the producer sets it but before it signals. So a third sneaky goroutine can’t change pokemon until producer unlocks,
			ensuring the signal corresponds to the state. */
			cond.Signal() // Wake first consumer goroutine in the queue
			cond.L.Unlock()
		}
	}()

	wg.Wait()
}
