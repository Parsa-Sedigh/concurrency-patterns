package main

import (
	"bytes"
	"fmt"
	"runtime"
	"strings"
	"sync"
)

/*
	sync.Pool - a cache of temporary objects, to stop allocating the same short-lived thing over and over.

	It is NOT a connection pool, NOT a worker pool, and NOT a way to limit anything. It has one job: reuse
	garbage so the GC has less to collect. If the objects are expensive to create AND must stay alive, you want
	your own pool with a mutex, not this.

	THE TEMPLATE - four lines, and every one of them matters:

		buf := bufPool.Get().(*bytes.Buffer)   // 1. Get, and type-assert (it returns `any`)
		defer func() {
			buf.Reset()                        // 3. clean it before it goes back
			bufPool.Put(buf)                   // 4. Put it back
		}()
		... use buf ...                        // 2. use it

	Three rules:

	 1. Get() gives you either a NEW object or a USED one. You cannot tell which. So the object must be reset
	    before anyone reads it - either right after Get or right before Put, but pick one and always do it.

	 2. Put a POINTER, not a value. Put takes `any`, so putting a struct by value allocates a box on the heap,
	    which is the exact thing you were trying to avoid.

	 3. Anything in the pool can disappear at any moment. The GC empties pools. Never put something in that you
	    would be sad to lose, and never assume Get returns what you last Put.
*/

var bufPool = sync.Pool{
	// New is called only when the pool is empty. If you leave it nil, Get returns nil and you have to handle it.
	New: func() any {
		return new(bytes.Buffer)
	},
}

// join is the shape you would actually write: Get, defer the cleanup, use, return a copy.
func join(parts []string) string {
	buf := bufPool.Get().(*bytes.Buffer)

	defer func() {
		/* Reset before Put, so the pool holds clean buffers. Resetting right after Get works too and is more
		defensive (a buggy Put elsewhere cannot poison you), but then the pool sits on dirty data. Either is
		fine. Doing neither is the bug in demoResetBug below.

		Note Reset() only sets the length to 0 - it keeps the underlying array, which is the entire point.*/
		buf.Reset()
		bufPool.Put(buf)
	}()

	for i, p := range parts {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(p)
	}

	// buf.String() copies. Returning buf.Bytes() would hand out memory that is about to be reused by someone else.
	return buf.String()
}

// joinNoPool is the same work with a fresh buffer every time, for comparison.
func joinNoPool(parts []string) string {
	var buf bytes.Buffer

	for i, p := range parts {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(p)
	}

	return buf.String()
}

// ---------------------------------------------------------------- demos

func allocs(name string, f func()) {
	var before, after runtime.MemStats

	runtime.GC()
	runtime.ReadMemStats(&before)

	f()

	runtime.ReadMemStats(&after)

	fmt.Printf("  %-16s %8d allocations, %6d KB total\n",
		name, after.Mallocs-before.Mallocs, (after.TotalAlloc-before.TotalAlloc)/1024)
}

func demoAllocations() {
	parts := strings.Split(strings.Repeat("value,", 200), ",")

	const rounds = 2000

	fmt.Println("1. what the pool actually buys you")

	allocs("without pool", func() {
		for range rounds {
			_ = joinNoPool(parts)
		}
	})

	allocs("with pool", func() {
		for range rounds {
			_ = join(parts)
		}
	})

	fmt.Println("  the saving is the buffer's internal array - the returned string is still allocated each time")
}

// The bug that costs people the interview: Get() can hand you a used object, so skipping the reset leaks the
// previous caller's data into your result.
func demoResetBug() {
	fmt.Println("\n2. why the reset is not optional")

	var pool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

	broken := func(s string) string {
		buf := pool.Get().(*bytes.Buffer)
		defer pool.Put(buf) // no Reset

		buf.WriteString(s)

		return buf.String()
	}

	/* Counted over several calls rather than shown once, because whether Get hands back the used buffer or a
	brand new one is not something you control - the runtime may have dropped it, or moved this goroutine to
	another CPU. That unpredictability IS the lesson: you cannot tell what you got, so you must always reset.*/
	polluted := 0

	for i := range 10 {
		want := fmt.Sprintf("call%d", i)
		if got := broken(want); got != want {
			polluted++
		}
	}

	fmt.Printf("  %d of 10 calls came back with an earlier call's data still attached\n", polluted)
	fmt.Println("  one Reset() anywhere in that function makes it 0 of 10")
}

// sync.Pool is a cache, not storage. The GC empties it. This is the reason it cannot be a connection pool:
// your connections would silently vanish, and there is no hook to Close them when they do.
func demoGCClearsPool() {
	fmt.Println("\n3. the pool is emptied by the GC")

	created := 0

	var pool = sync.Pool{
		New: func() any {
			created++
			return new(bytes.Buffer)
		},
	}

	pool.Put(new(bytes.Buffer))
	pool.Put(new(bytes.Buffer))

	_ = pool.Get() // came from the pool, New not called
	fmt.Println("  after Put x2 then Get: New called", created, "times")

	/* Two GCs, because since Go 1.13 a pool keeps a "victim cache": the first GC moves items aside instead of
	dropping them, so they survive one cycle. The second GC is what actually clears them.*/
	runtime.GC()
	runtime.GC()

	_ = pool.Get()
	fmt.Println("  after 2 GCs then Get:  New called", created, "times  <- everything we Put is gone")
}

func demoConcurrentUse() {
	fmt.Println("\n4. it is safe for concurrent use, and that is where it pays off")

	var wg sync.WaitGroup

	for i := range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range 1000 {
				_ = join([]string{"goroutine", fmt.Sprint(i)})
			}
		}()
	}

	wg.Wait()

	fmt.Println("  8 goroutines x 1000 joins, no mutex anywhere - Get/Put are per-CPU and almost never contend")
}

func main() {
	demoAllocations()
	demoResetBug()
	demoGCClearsPool()
	demoConcurrentUse()
}

/*
	INTERVIEW NOTES

	What is it for?
	  Cutting allocations of short-lived, interchangeable objects that you create constantly: byte buffers,
	  encoders, scratch slices. The standard library uses it in fmt, encoding/json and net/http exactly this way.

	When NOT to use it:
	  - Database connections, files, sockets. Items vanish without warning and there is no callback, so nothing
	    gets Closed. Use a real pool with a mutex, or database/sql, which already is one.
	  - Limiting concurrency. A pool has no size and no bound. Use a buffered channel or errgroup.SetLimit.
	  - Objects that are expensive to build and must survive. The GC does not care how expensive they were.
	  - Anything where you need to know how many objects exist. There is no Len().

	Questions you should expect:

	  "Why does Get return `any`?"
	    It predates generics and the API is frozen for compatibility. You always type-assert. Getting the
	    assertion wrong panics, which is why the New function and the assertion must agree.

	  "What happens if New is nil?"
	    Get returns nil when the pool is empty. Then your type assertion panics. Always set New.

	  "Is it safe for concurrent use?"
	    Yes, and that is the point. Internally each P (roughly, each CPU) has its own private slot plus a shared
	    queue, so Get and Put usually touch no shared state and take no lock. Under contention a P can steal from
	    another P's shared queue.

	  "Why must you not copy a sync.Pool?"
	    It carries per-P state and a noCopy marker. Copying it after first use breaks that state - go vet catches
	    it. Pass it as a pointer or keep it in a package-level var.

	  "How do you stop the pool holding huge objects forever?"
	    Cap what you put back:

	        const maxKeep = 64 << 10
	        if buf.Cap() <= maxKeep {
	            buf.Reset()
	            bufPool.Put(buf)
	        }

	    Without this, one 10MB request leaves a 10MB buffer in the pool, and with N CPUs you can end up holding
	    N of them. Reset() sets length to 0 but keeps the array, so it does not help here.

	  "Does it reduce GC pressure or memory usage?"
	    GC pressure - fewer allocations and less work for the collector. Peak memory usually goes UP a little,
	    because pooled objects stay alive between uses.

	  "When is it not worth it?"
	    When the object is small and cheap. Get, the type assertion and Put are not free, and Go's allocator is
	    fast. Measure with -benchmem before reaching for it. This is the one place where "profile first" is not
	    a platitude: pooling a small struct is regularly slower than just allocating it.

	The mistakes that actually happen:
	  - Forgetting the reset, so the previous caller's data leaks into yours. Demo 2.
	  - Returning buf.Bytes() instead of buf.String(), handing out memory that is about to be reused.
	  - Putting a value instead of a pointer, which allocates a box and defeats the purpose.
	  - Using it as a connection pool. Demo 3.
	  - Putting back an object you still hold a reference to, then writing through that reference.
*/
