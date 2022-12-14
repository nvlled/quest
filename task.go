package quest

import (
	"errors"
	"log"
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

var WarnDisabled = "[WARN] Task is used while it is freed on pool. This could lead to unexpected behavior."

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

	Yield() bool

	// Similar to Await(), but will not panic
	// even if called with SetPanic(true).
	Anticipate() (result T, valid bool)

	// Resets the task, making the task available again for
	// Resolve(), Cancel() and Error().
	// Clears the errors if any.
	// Sets panic to false.
	// success is false if no effect is done.
	Reset() (success bool)

	// Calls Await() then Reset()
	// Blocks the thread until it is available.
	AwaitAndReset() (result T, valid bool)

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

	// Set value to true to make Await()
	// panic when Cancelled(). False by default.
	// Note: Only affects Await(), nothing else.
	// Note: it will panic(ErrCanceled), not
	// the error from Fail().
	SetPanic(value bool)
}

var idGen atomic.Int64

// A void task represents tasks that doesn't
// return any result.
type VoidTask = Task[Void]

type taskImpl[T any] struct {
	id int64

	value        T
	defaultValue T
	status       atomic.Int32

	awaitMu   sync.RWMutex
	resolveMu sync.Mutex

	err error

	panicOnCancel atomic.Bool

	// A flag used to indicate if the task
	// is currently freed in the task pool.
	disabled atomic.Bool
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
	if task.disabled.Load() {
		log.Print(WarnDisabled)
		return
	}

	if task.status.Load() != taskPending {
		return
	}

	task.resolveMu.Lock()

	if task.status.Load() != taskPending {
		task.resolveMu.Unlock()
		return
	}

	task.value = value
	task.status.Store(taskResolved)
	task.awaitMu.Unlock()
	task.resolveMu.Unlock()

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

	if task.status.Load() != taskPending {
		task.resolveMu.Unlock()
		return false
	}

	task.status.Store(taskCanceled)
	task.awaitMu.Unlock()
	task.resolveMu.Unlock()

	return true
}

func (task *taskImpl[T]) IsCancelled() bool {
	return task.status.Load() == taskCanceled
}

func (task *taskImpl[T]) IsDone() bool {
	return task.status.Load() != taskPending
}

func (task *taskImpl[T]) SetPanic(value bool) {
	if task.disabled.Load() {
		log.Print(WarnDisabled)
		return
	}
	task.panicOnCancel.Store(value)
}

func (task *taskImpl[T]) Await() (T, bool) {
	if task.status.Load() == taskPending {
		task.awaitMu.RLock()
		//lint:ignore SA2001 Donkeys
		task.awaitMu.RUnlock()
	}
	if task.status.Load() == taskCanceled && task.panicOnCancel.Load() {
		panic(ErrCancelled)
	}

	return task.getValue(), task.status.Load() == taskResolved
}

func (task *taskImpl[T]) Anticipate() (T, bool) {
	if task.status.Load() == taskPending {
		task.awaitMu.RLock()
		//lint:ignore SA2001 Donkeys
		task.awaitMu.RUnlock()
	}
	return task.getValue(), task.status.Load() == taskResolved
}

func (task *taskImpl[T]) Yield() bool {
	_, ok := task.Await()
	if !ok {
		return false
	}

	task.resolveMu.Lock()
	defer task.resolveMu.Unlock()

	task.awaitMu.Lock()
	if task.status.Load() != taskCanceled {
		task.status.Store(taskPending)
	}
	task.value = task.defaultValue

	return true
}

func (task *taskImpl[T]) AwaitAndReset() (T, bool) {
	value, ok := task.Await()
	task.Reset()
	return value, ok
}

func (task *taskImpl[T]) Reset() bool {
	if task.disabled.Load() {
		log.Print(WarnDisabled)
		return false
	}
	task.resolveMu.Lock()
	defer task.resolveMu.Unlock()

	if task.status.Load() == taskPending {
		return false
	}

	task.awaitMu.Lock()
	task.status.Store(taskPending)
	task.value = task.defaultValue
	task.err = nil
	task.panicOnCancel.Store(false)
	return true
}

func (task *taskImpl[T]) enable() {
	task.resolveMu.Lock()
	defer task.resolveMu.Unlock()
	task.disabled.Store(false)
}
func (task *taskImpl[T]) disable() {
	task.resolveMu.Lock()
	defer task.resolveMu.Unlock()
	task.disabled.Store(true)
}

func (task *taskImpl[T]) getValue() T {
	task.resolveMu.Lock()
	defer task.resolveMu.Unlock()
	return task.value
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
