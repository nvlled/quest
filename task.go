package quest

import (
	"errors"
	"sync"
	"sync/atomic"
)

// A type representing none.
// Used on tasks that doesn't return
// value: Task[Void]
type Void struct{}

// That value that represents nothing.
// Similar to nil, but safer.
var None = Void{}

// When task is called with SetPanic(true),
// this error is thrown (panic) while blocked
// on Await().
//
// Note: no other methods throw this error.
var ErrCancelled = errors.New("task cancelled while await")

type taskStatus = int32

const (
	taskPending  taskStatus = 0
	taskResolved taskStatus = 1
	taskCanceled taskStatus = 2
)

// A read-only interface of the Task.
// Used on AwaitAll, AwaitSome, and other AwaitN
// functions.
type Awaitable[T any] interface {
	// Waits for the result to finish.
	// Returns false if it failed or was cancelled.
	// Blocks the thread until it is available.
	Await() (T, bool)
}

type Task[T any] interface {
	// Mostly used for debugging.
	ID() int64

	// Waits for task to finish, and returns a result.
	// valid is false if it failed or was cancelled.
	// Blocks the thread until it is available.
	Await() (result T, valid bool)

	// Resets the task, making the task available again for
	// Resolve(), Cancel() and Error().
	// Clears the errors if any.
	// Sets panic to false.
	// success is false if no effect is done.
	Reset() (success bool)

	// Resolves the task result.
	// No effect if task is already Resolve() or Cancel(),
	// unless Reset() is called.
	Resolve(result T)

	// Cancels the task.
	Cancel()

	// Cancel() the task, then sets the error.
	// The error can be retrieved with Error()
	Fail(error)

	// Returns the error set by Fail().
	// returns nil if there is none.
	Error() error

	// Returns true if Cancel() or Fail() is called.
	IsCancelled() (done bool)

	// Returns true if Resolve(), Cancel() or Fail() is called.
	IsDone() (done bool)
}

var idGen atomic.Int64

// A void task represents tasks that doesn't
// return any result.
type VoidTask = Task[Void]

type taskImpl[T any] struct {
	id int64

	value        T
	defaultValue T
	status       taskStatus

	awaitMu   sync.RWMutex
	resolveMu sync.Mutex

	err error
}

// Regular functions that returns (T, bool)
// are also Awaitable.
type AwaitableFn[T any] func() (T, bool)

func (fn AwaitableFn[T]) Await() (T, bool) {
	return fn()
}

func newTask[T any]() *taskImpl[T] {
	t := &taskImpl[T]{}
	t.awaitMu.Lock()
	t.id = idGen.Add(1)
	return t
}

// Creates a new task
// Example:
//
//	NewTask[int]()
//	NewTask[string]()
//	NewTask[Event]()
func NewTask[T any]() Task[T] {
	return newTask[T]()
}

// Creates a new void task
// Equivalent to NewTask[Void]()
// Void tasks are resolved with None,
// e.g. NewVoidTask().Resolve(None)
func NewVoidTask() VoidTask {
	return newTask[Void]()
}

// Start the function fn, and returns a task.
// The task is Resolve() when fn returns.
// The resolved value is what fn returns.
// Note: it does not use the default pool.
// Example:
//
//	func compute() int {
//	  // do some lone running operations, then
//	  return 2+2
//	}
//	n := Start(compute).Await() // n == 4
func Start[T any](fn func() T) Task[T] {
	task := NewTask[T]()
	go func() {
		task.Resolve(fn())
	}()
	return task
}

func (task *taskImpl[T]) ID() int64 {
	return task.id
}

func (task *taskImpl[T]) Resolve(value T) {
	task.resolveMu.Lock()
	defer task.resolveMu.Unlock()

	if task.status != taskPending {
		return
	}

	task.value = value
	task.status = taskResolved
	task.awaitMu.Unlock()

}

func (task *taskImpl[T]) Error() error {
	return task.err
}

func (task *taskImpl[T]) Fail(err error) {
	if task.cancel() {
		task.err = err
	}
}

func (task *taskImpl[T]) Cancel() {
	task.cancel()
}

func (task *taskImpl[T]) cancel() bool {
	task.resolveMu.Lock()
	defer task.resolveMu.Unlock()

	if task.status != taskPending {
		return false
	}

	task.status = taskCanceled
	task.awaitMu.Unlock()

	return true
}

func (task *taskImpl[T]) IsCancelled() bool {
	task.resolveMu.Lock()
	defer task.resolveMu.Unlock()
	return task.status == taskCanceled
}

func (task *taskImpl[T]) IsDone() bool {
	task.resolveMu.Lock()
	defer task.resolveMu.Unlock()
	return task.status != taskPending
}

func (task *taskImpl[T]) Await() (T, bool) {
	task.resolveMu.Lock()
	if task.status == taskPending {
		task.resolveMu.Unlock()
		task.awaitMu.RLock()
		//lint:ignore SA2001 Donkeys
		task.awaitMu.RUnlock()
	} else {
		task.resolveMu.Unlock()
	}

	task.resolveMu.Lock()
	defer task.resolveMu.Unlock()

	return task.value, task.status == taskResolved
}

func (task *taskImpl[T]) Reset() bool {
	task.resolveMu.Lock()
	defer task.resolveMu.Unlock()

	if task.status == taskPending {
		return false
	}

	task.awaitMu.Lock()
	task.status = taskPending
	task.value = task.defaultValue
	task.err = nil

	return true
}

// Waits for all tasks or awaitables to finish.
// Returns nil for tasks that have been cancelled.
// The tasks can have different result types.
// Blocks until all tasks are resolved or cancelled.
// Check for nils before derefencing the pointers.
// Example:
//
//	var task1 = NewTask[int]()
//	task1.Resolve(10)
//	var task2 AwaitableFn[string]= func() (string, bool) { return "apples", true }
//	a, b := Await2(task1, task2)
//	// a == 10, b == "apples"
func Await2[A any, B any](t1 Awaitable[A], t2 Awaitable[B]) (*A, *B) {
	return asPointer(t1.Await()), asPointer(t2.Await())
}

// Same behaviour with Await2()
func Await3[A any, B any, C any](t1 Awaitable[A], t2 Awaitable[B], t3 Awaitable[C]) (*A, *B, *C) {
	return asPointer(t1.Await()), asPointer(t2.Await()), asPointer(t3.Await())
}

// Same behaviour with Await2()
func Await4[A any, B any, C any, D any](
	t1 Awaitable[A],
	t2 Awaitable[B],
	t3 Awaitable[C],
	t4 Awaitable[D],
) (*A, *B, *C, *D) {
	return asPointer(t1.Await()),
		asPointer(t2.Await()),
		asPointer(t3.Await()),
		asPointer(t4.Await())
}

// Same behaviour with Await2()
func Await5[A any, B any, C any, D any, E any](
	t1 Awaitable[A],
	t2 Awaitable[B],
	t3 Awaitable[C],
	t4 Awaitable[D],
	t5 Awaitable[E],
) (*A, *B, *C, *D, *E) {
	return asPointer(t1.Await()),
		asPointer(t2.Await()),
		asPointer(t3.Await()),
		asPointer(t4.Await()),
		asPointer(t5.Await())
}

// Same behaviour with Await2(), except
// the result is not return, and the tasks must have
// the same types.
// The result can be checked afterwards with Await().
// Example:
//
//	var task1 = NewTask[int]()
//	var task2 = NewTask[int]()
//	var task3 AwaitableFn[int]= func() (string, bool) { return 0, true }
//	AwaitAll(task1, task2, task3)
func AwaitAll[T any](tasks ...Awaitable[T]) {
	for _, t := range tasks {
		t.Await()
	}
}

// Waits for one task to complete.
// It blocks until at least one task has
// been Resolved() or Cancel().
//
//	var task1 = NewTask[int]()
//	var task2 = NewTask[int]()
//	var task3 AwaitableFn[int]= func() (string, bool) { return 0, true }
//	AwaitSome(task1, task2, task3)
func AwaitSome[T any](tasks ...Awaitable[T]) {
	blocker := AllocTask[Void]()
	defer FreeTask(blocker)

	for _, t := range tasks {
		if blocker.IsDone() {
			break
		}
		go func(t Awaitable[T]) {
			t.Await()
			if !blocker.IsDone() {
				blocker.Resolve(None)
			}
		}(t)
	}

	blocker.Await()
}
