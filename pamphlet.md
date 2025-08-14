## 1-Data race & race condition
### Data race - Unsynchronized Memory Access

A data race happens when two or more threads (or goroutines in Go) access the same memory location at the same time,
and at least one of them is writing, without any protection. This causes unpredictable results because the operations clash.
**So one goroutine is reading and another is writing.**

- Think of it as: Two people trying to **write** on the same whiteboard **at once** without taking turns—chaos!
- Key Point: It’s about **how** the data is accessed (**no synchronization**).

NOTE: Critical section is the place in your code that has access to a shared memory.

### Race Condition: Timing Matters

A race condition is when a program’s behavior depends on the timing or order of events (like which thread runs first).
It’s broader than a data race and can happen even if memory access is protected. So we could still have race condition while
data race is mitigated.

- Think of it as: Two people racing to push a button, and the outcome depends on who gets there first.
- Key Point: It’s about when things happen (order/timing).

### Summary
- Data Race: Chaos from unprotected memory access (e.g., two writers clashing). Add a lock (like Mutex) or use channels to control access.
- Race Condition: Unexpected results from timing/order (e.g., who finishes first). Design the program so the 
order doesn’t matter, or use tools like `WaitGroup` properly.

## 2-mutex vs RWMutex
### sync.Mutex
- Locks entire resource; only one goroutine (reader or writer) can access at a time.
- Use when: Any access (read or write) needs exclusivity.

### sync.RWMutex:
- Allows multiple readers (read locks) simultaneously, but only one writer (write lock).
So RWMutex allows concurrent reads(but no writes simultaneously), making it faster for many observers.
- Use when: Many goroutines read, few write, and reads can happen concurrently.

The code demonstrates that RWMutex is faster when there are many readers(observers), as they can read concurrently, while sync.Mutex serializes both
reads and writes.

- The code shows how RWMutex scales better with many readers, as multiple observers can read simultaneously.
- Full mutex(sync.Mutex) forces all accesses (even reads) to be sequential, slowing down as observer count grows.

### RWMutex vs. Mutex
- sync.Mutex: Only one goroutine (producer or observer) can hold the lock at a time.
This creates contention, especially with many observers.
- sync.RWMutex: Allows **multiple** readers (observers) to hold read locks simultaneously, 
but writers (producer) need an exclusive write lock. This reduces contention **when most operations are reads**.

### Memory Aids
- Producer: Think of a chef (writer) locking the kitchen to cook (write lock), doing it 5 times with a quick nap.
- Observer: Think of waiters (readers) peeking at the menu (read lock), which multiple waiters can do together with RWMutex.

Another terminology:
- RWMutex vs. Mutex: RWMutex is like a library where many can read books at once, but only one can write. 
- Mutex is like a single-room library where only one person (reader or writer) is allowed in.

## 3-sync.cond (condition variables)
Signals aren't guaranteed to mean the condition is still true by the time a waiting goroutine wakes up.
This is why you must always check the condition in a loop:
```go
package main

func main() {
  c.L.Lock()
  
  for !condition() {
    c.Wait()
  }
  
  // ... make use of condition ...
  c.L.Unlock()
}
```

### "Why not just use c.Wait() directly without a loop?”
When Wait() returns, we can’t just assume that the condition we’re waiting for is immediately true. While our goroutine is
waking up, other goroutines could’ve messed with the shared state and the condition might not be true anymore. 
So, to handle this properly, we always want to use Wait() inside a loop.

Notes:
- If there are multiple goroutines waiting, Signal() wakes up the first one in the queue.
- Calling `cond.Wait()` immediately unlocks the mutex, so the mutex must be **locked** **before** calling `cond.Wait()`, 
  otherwise, it will panic.
- The idea here is that Signal() is used to wake up one goroutine and tell it that the condition **might** be satisfied. 
- When the waiting goroutine gets woken up (by Signal() or Broadcast()), it doesn’t immediately resume work.
First, it has to re-acquire the lock (Lock()).
- When `Broadcast()` is called, it marks all the waiting goroutines as ready to run, but they don’t run immediately,
they’re picked based on the Go scheduler’s underlying algorithm, which can be a bit unpredictable.

### Why we need a for loop when waiting on the condition?
Since there's a gap between signaling and actually waking up. The flow of the code is:

- Producer Goroutine: Every 1 millisecond (ms), it:
    - Locks the mutex.
    - Sets pokemon to a random name
    - Unlocks the mutex.
    - Calls `cond.Signal()` to notify any waiting goroutines: "Hey, pokemon changed, check it!"
- Consumer Goroutine: Waits for pokemon == "Pikachu":
    - Locks the mutex.
    - In a loop: If not "Pikachu", calls `cond.Wait()` (pauses until signaled, unlocking the mutex while waiting).
    - When woken, rechecks the condition (loop ensures this). 
    - If true, prints "Caught Pikachu" and unlocks.
    - Without the loop, the consumer would wrongly assume it's "Pikachu" and proceed (bug!).
      With the loop, it sees the condition is false and goes to waiting again. Which means unlocking the mutex and suspending that goroutine.

So:
- No loop = assumes wake means true (wrong, due to gap).
- Loop = resilient to changes during the gap.

### 1. Gap between `cond.Signal()` and actually waking up
The gap is between these two:

- The producer calling `cond.Signal()` (after setting pokemon).
- The consumer goroutine **actually** waking up from `cond.Wait()` and re-locking the mutex to check the condition.

During this gap (even if tiny, like microseconds), things can go wrong in 2 ways:
1. The Producer Changes pokemon **Again**:
  - We know the producer runs in a fast loop (every 1ms).
  - Suppose at time T=0ms: Producer sets pokemon = "Pikachu"(which is what the consumer is looking for) and signals.
  - Signal wakes the consumer... but the consumer might not wake instantly (scheduling delays in Go's runtime or OS).
  - At T=1ms: Producer loops again, sets pokemon = "Charmander", and signals **again**(so pokemon changed before consumer taking any action).
  - Consumer finally wakes at T=1.5ms: It re-locks the mutex, checks pokemon, but now it's "Charmander"—not "Pikachu"!
  - Without the loop, the consumer would wrongly assume it's "Pikachu" and proceed (bug!).
    With the loop, it sees the condition is false and waits again.
2. Other Goroutines Modify the Shared State:
  - Imagine a third goroutine (e.g., another producer or modifier) that locks the mutex during the gap and changes pokemon to something else.
  - Same issue: Signal says "change happened," but by the time it wakes up, the state isn't what the consumer expected.

### 2. Gap between producer's `cond.L.Unlock()` and consumer's cond.Wait()
There’s a potential window where another goroutine could acquire the mutex and modify pokemon after the producer unlocks but
before the consumer calls `cond.Wait()`.

The for loop in the consumer (for pokemon != "Pikachu") protects against state changes in this window, 
just as it does for the gap between calling cond.Signal() and cond actually waking up.

### Solution to eliminate these two gaps
Calling `Signal()` before Unlock() in the producer eliminates the gap between Unlock() and Signal(),
making the signal more reliable by ensuring it matches the state change.

The only remaining gap is post-`Unlock()` to consumer wake-up(the delay when consumer actually wakes up) which the for loop handles.