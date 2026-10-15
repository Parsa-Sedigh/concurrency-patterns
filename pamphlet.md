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

### IMPORTANT: the for loop does NOT handle that gap
The line above is wrong, and `3-cond/main.go` proves it: that program deadlocks 35 runs out of 40.

The for loop stops the consumer from acting on a **false** condition. It cannot recover a condition that was
true and got **overwritten** while the consumer was asleep. In that file the producer set `pokemon = "Pikachu"`
about 20 times per run and the consumer caught none of them: `Signal()` only marks the consumer runnable, the
producer re-locks immediately, and it finishes all 100 iterations before the consumer acquires the mutex once.
The consumer then sees some other pokemon, calls `Wait()` again, and nothing is left alive to signal it.

**A signal is not a message.** Signals are not queued, not counted and not buffered. The waiter reads whatever
the state holds when it finally gets the lock. So the thing you wait on must be **durable state** that stays
true until a waiter consumes it:

```go
if pokemon == "Pikachu" {
    sawPikachu = true // sticky, so waking late is fine
}
...
for !sawPikachu {     // instead of: for pokemon != "Pikachu"
    cond.Wait()
}
```

Three rules, not two:
1. Both sides hold the same lock.
2. `for`, never `if`.
3. The condition must be durable state, not a value someone keeps overwriting.

Rules 1 and 2 were already correct in that file. Rule 3 is what it broke. Full write-up in `3-cond/README.md`.

## 4-fan-in-fan-out 
In Go concurrency, the fan-in pattern refers to a scenario where multiple goroutines send **results** to a single **channel**,
effectively "funneling" their outputs into one stream of data that can be **consumed by a single receiver.**

fan-out: a single source distributes work to multiple **workers**.

### Choosing the Right Approach for worker pool
- Static Division: Use when the task count is known upfront and tasks have similar durations. It guarantees equal distribution.
- Dynamic Division: Use for dynamic tasks. Use a buffered channel to reduce contention and improve fairness.
- Round-Robin: Use when strict equality is critical, and you’re okay with managing multiple channels. 
- Work Stealing: Use for advanced scenarios with highly variable task durations (not shown here due to complexity).

### Static division - When number of tasks are known up-front
- Worker Assignment: Each worker goroutine receives its own slice of tasks (taskChunk) and processes them directly, 
eliminating the need for a shared task channel.
- Fan-In for Results: Workers send results to a **shared** resultCh channel.
- Equal Distribution: Each worker gets an equal (or nearly equal) number of tasks. If there’s a remainder, the first few workers get one extra task.

Pros:
- Guarantees equal task distribution (or as equal as possible with remainders).
- No contention on a task channel, as tasks are pre-assigned.
- Simple to implement for a known task count.

Cons:
- Requires knowing the task list upfront.
- Less flexible for dynamically generated tasks.

### Use a task channel when
1. You don't have all the tasks up front. Tasks arrive from a scanner, a network connection, a database cursor, another stage of
a pipeline. You cannot split a slice you haven't finished building. This is the strongest reason, and often the only one you need.
2. Tasks take very different amounts of time. A channel balances load by itself: whichever worker finishes first takes the next task. 
Split a slice into fixed chunks instead and one worker can get all the slow items while the others sit idle.
3. You want the producer to slow down. An unbuffered or small-buffered channel blocks the sender when every worker is busy.
4. Several producers feed one pool, or the pool outlives one batch — a long-running server where handlers push work in.

### Dont use task chan when
When you already have the full slice and the tasks are cheap or uniform. Split it:

### Note: When to Make tasksCh Buffered?
1. High Task Volume or **Burst** of Tasks
2. Reducing **Contention** Between Distributor and Workers: With an unbuffered channel, the distributor goroutine blocks on
   each send (tasksCh <- task) until a worker receives the task. This can create contention if many workers are competing for 
   tasks or if task processing times vary significantly.
3. Improving **Fairness** in Task Distribution: With an unbuffered channel, faster workers may grab tasks more quickly, leading to uneven task distribution.
   A buffered channel allows tasks to be queued, giving slower workers a chance to pick up tasks before the channel empties.
   Note that in static division we don't need buffered chan because tasks are already split evenly
   between workers
4. **Asynchronous** Task **Generation**: If tasks are generated dynamically (e.g., from an external source like a network or stream),
   a buffered channel can absorb bursts of incoming tasks, preventing the producer from blocking while workers catch up.

## 5-Mutex vs WaitGroup vs Channel vs Cond
These four are not alternatives to each other. Each answers a different question, and most real code uses
several at once. Pick by the question you are actually asking.

| tool | the question it answers | use it when |
| --- | --- | --- |
| `sync.Mutex` | "May I touch this state right now?" | Two or more goroutines read and write the same variable, and you want them to take turns. |
| `sync.WaitGroup` | "Have they all finished?" | You started N goroutines and must not continue until every one is done. |
| `chan T` | "Who wants this value?" | You are moving data between goroutines, or releasing waiters once with `close`. |
| `sync.Cond` | "Wake me when this might be true." | You are waiting on a predicate over state a mutex already guards, and the predicate becomes true again and again. |

### Mutex - protect state
Guards a **critical section**: the lines that read or write shared memory. It says nothing about order or
timing, only that one goroutine is inside at a time. If you find yourself sleeping in a loop to wait for a value
to change while holding a mutex, you want a Cond (or a channel), not a mutex.

Cannot: wait for anything, carry values, or be cancelled.

### WaitGroup - wait for a known count
`Add` before you start the goroutine, `Done` inside it (use `defer`), `Wait` somewhere else. The count must be
known - that is the whole limitation. `Add` inside the goroutine is a bug, because `Wait` can see zero and
return before the goroutine has even started.

A WaitGroup tells you goroutines have **finished**. It says nothing about what they produced, and nothing about
the order they finished in.

Cannot: be cancelled, return values, or handle an unknown number of goroutines added over time.

### Channel - move values, and release waiters once
Does four separate jobs, which is why it is the default answer in Go:
1. Hand a value to whichever goroutine is ready: `ch <- v`.
2. Release **any number** of waiters, one time only: `close(ch)`. The signaller never has to know how many
   there are, and goroutines arriving after the close are released too.
3. Act as a bounded queue: `make(chan T, n)`.
4. Be waited on **with a timeout or cancellation**, via `select` with `ctx.Done()`. Nothing else here can do this.

Cannot: repeat a broadcast. A channel closes once. Conditions that flip true, false, true need a Cond or a
fresh channel per round.

### Cond - wait for a repeatable condition on locked state
Always used **together with** a mutex, never instead of one. Worth it only when all three hold:
1. The state is already behind a mutex.
2. The condition becomes true more than once.
3. Waiters must re-check on waking, because they compete for the state.

Cannot: be cancelled, deliver values, or preserve a signal for a waiter that has not arrived yet.

### Deciding in one pass
- Only protecting shared state? **Mutex**.
- Only waiting for N goroutines to end? **WaitGroup**.
- Moving values between goroutines? **Channel**.
- One-time "go ahead" to an unknown number of goroutines? **`close(ch)`**, not Cond.
- Need a timeout or ctx cancellation while waiting? **Channel** - Cond cannot be interrupted at all.
- Waiting on a condition over mutex-guarded state that keeps flipping? **Cond**.

### How they combine
- **Mutex + Cond**: always together. `sync.NewCond(&mu)` takes the mutex that already guards the state.
- **WaitGroup + Channel**: the standard "close the results channel" pattern - one goroutine does
  `wg.Wait(); close(resultCh)` while the main goroutine ranges over `resultCh`. `wg.Wait()` must be in its own
  goroutine or it deadlocks, because the workers cannot finish until someone reads the channel. See
  `worker-pool/main.go` and `merge()` in `fan-in-fan-out/main.go`.
- **Mutex + WaitGroup**: workers append results under a mutex, `Wait` tells the caller when it is safe to read
  the slice without the lock.
- **Channel + ctx**: pair every **send** with `case <-ctx.Done()`. Receives usually do not need it, because the
  upstream `close` already ends a `for range`. See `worker-pool/receiving-tasks.md`.

### Two mistakes that show up in interviews
- Using a **Mutex plus a sleep loop** to wait for a value to change. That is a Cond or a channel.
- Using a **Cond for a one-shot event** with an unknown number of waiters. That is `close(ch)`, which is shorter
  and can be cancelled.