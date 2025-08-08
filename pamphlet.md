## Data race & race condition
### Data race - Unsynchronized Memory Access

A data race happens when two or more threads (or goroutines in Go) access the same memory location at the same time, 
and at least one of them is writing, without any protection. This causes unpredictable results because the operations clash.

- Think of it as: Two people trying to **write** on the same whiteboard **at once** without taking turns—chaos!
- Key Point: It’s about **how** the data is accessed (**no synchronization**).

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