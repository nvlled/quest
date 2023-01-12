package quest_test

import (
	"math/rand"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nvlled/quest"
)

func TestUsage(t *testing.T) {
	// Create three tasks
	t1 := quest.NewTask[int]()
	t2 := quest.NewTask[int]()
	t3 := quest.NewTask[int]()

	// t1 will do some computations
	go func() {
		n := 0
		for i := 0; i < 10000; i++ {
			n += i
		}
		t1.Resolve(n)
	}()

	// t2 will do some computations using
	// t1's result
	go func() {
		// Await() can be called multiple times,
		// it will return the same result
		// since the last Resolve()
		n, ok := t1.Await()

		if !ok {
			// t1 is already closed
			return
		}

		for i := 10000; i < 20000; i++ {
			n += i
		}
		t2.Resolve(n)
	}()

	go func() {
		n, ok1 := t1.Await()
		m, ok2 := t2.Await()
		if !ok1 || !ok2 {
			// t1 or t2 is already closed
			return
		}
		t3.Resolve(n * m)
	}()

	go func() {
		// computations took too long, cancel the tasks
		time.Sleep(100 * time.Millisecond)
		if t1.IsDone() && t2.IsDone() && t3.IsDone() {
			return
		}

		// note, this will not have any effect
		// if the tasks are already done
		t1.Cancel()
		t2.Cancel()
		t3.Cancel()

		// Reset the tasks for another use
		t1.Reset()
		t2.Reset()
		t3.Reset()

		// Resolve with these values
		t1.Resolve(-1)
		t2.Resolve(-2)
		t3.Resolve(-3)
	}()

	// Await all three tasks to finish
	r1, r2, r3 := quest.Await3[int, int, int](t1, t2, t3)
	if *r1 != 49995000 {
		t.Error()
	}
	if *r2 != 199990000 {
		t.Error()
	}
	if *r3 != 9998500050000000 {
		t.Error()
	}

}

func TestUsage2(t *testing.T) {
	task1 := quest.NewTask[int]()
	task2 := quest.NewTask[int]()

	go func() {
		// ... do some computation, then
		task1.Resolve(1000)
		task2.Resolve(2000)

		// resolving again won't have any effect
		task1.Resolve(3000) // doesn't work
	}()

	result1, _ := task1.Await()
	result2, _ := task2.Await()

	if result1 != 1000 || result2 != 2000 {
		t.Errorf("result1=%v, result2=%v", result1, result2)
	}
}

func TestResolve(t *testing.T) {
	t1 := quest.NewTask[int]()
	t2 := quest.NewTask[int]()
	done := false

	go func() {
		time.Sleep(1 * time.Millisecond)
		done = true
		t1.Resolve(123)
		t2.Resolve(456)
		t1.Cancel()
		t2.Cancel()
	}()

	value, ok := t1.Await()
	if value != 123 || !ok {
		t.Error("task 1 failed to resolve")
	}

	value, ok = t1.Await()
	if value != 123 || !ok {
		t.Error("task 1 failed to await again")
	}

	value, ok = t2.Await()
	if value != 456 || !ok {
		t.Error("task 2 failed to resolve")
	}

	if !done {
		t.Error("not yet done")
	}
}

func TestCancel(t *testing.T) {
	t1 := quest.NewTask[int]()

	go func() {
		t1.Cancel()
		t1.Resolve(0)
	}()

	_, ok := t1.Await()
	if ok {
		t.Error("task 2 failed to cancel")
	}
}

func TestAwait3(t *testing.T) {
	t1 := quest.NewTask[int]()
	t2 := quest.NewTask[int]()
	t3 := quest.NewTask[int]()

	go func() {
		t1.Resolve(111)
		t2.Cancel()
		t3.Resolve(333)
	}()

	t1Val, t2Val, t3Val := quest.Await3[int, int, int](t1, t2, t3)
	if *t1Val != 111 {
		t.Error("task 1 has wrong value")
	}
	if t2Val != nil {
		t.Error("task 2 should have nil")
	}
	if *t3Val != 333 {
		t.Error("task 3 has wrong value")
	}
}

func TestAwaitAll(t *testing.T) {
	t1 := quest.NewTask[int]()
	t2 := quest.NewTask[int]()
	t3 := quest.NewTask[int]()
	done := false

	go func() {
		done = true
		time.Sleep(10 * time.Millisecond)
		t1.Cancel()
		t2.Resolve(111)
		t3.Resolve(111)
	}()

	quest.AwaitAll[int](t1, t2, t3)
	if !done {
		t.Error("should block first")
	}
}

func TestAwaitSome(t *testing.T) {
	t1 := quest.NewTask[int]()
	t2 := quest.NewTask[int]()
	t3 := quest.NewTask[int]()
	done := false

	go func() {
		done = true
		t2.Resolve(111)
		t3.Cancel()
	}()

	quest.AwaitSome[int](t1, t2, t3)
	if !done {
		t.Error("should block first")
	}
}

func TestReset(t *testing.T) {
	t1 := quest.NewTask[int]()

	go func() {
		for {
			time.Sleep(1 * time.Millisecond)
			t1.Resolve(0)
		}
	}()
	go func() {
		for {
			time.Sleep(10 * time.Millisecond)
			t1.Resolve(0)
		}
	}()

	for i := 0; i < 100; i++ {
		t1.Await()
		t1.Reset()
	}
}

func TestConcurrency(t *testing.T) {
	t1 := quest.NewTask[int]()

	n := int32(500)
	counter := atomic.Int32{}
	go func() {
		for i := int32(0); i < n; i++ {
			t1.Await()
			go t1.Reset()
			go t1.Cancel()
			counter.Add(1)
		}
	}()

	for {
		randomSleep()
		t1.Resolve(1)
		go t1.Resolve(1)
		if counter.Load() == n {
			break
		}
	}
	time.Sleep(50 * time.Millisecond)
}

func randomSleep() {
	ms := 1 + rand.Int31n(999)
	time.Sleep(time.Duration(ms * int32(time.Microsecond)))
}
