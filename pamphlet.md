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