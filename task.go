package quest

import "sync"

type taskStatus int

type Unit struct{}

var None = Unit{}

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
	// Waits for task to finish, and returns a result.
	// valid is false if it failed or was cancelled.
	// Blocks the thread until it is available.
	Await() (result T, valid bool)

	// Resets the task, making the task available again for
	// Resolve(), Cancel() and Error().
	// Clears the errors if any.
	// success is false if no effect is done.
	Reset() (success bool)

	// Calls Await() then Reset()
	// Blocks the thread until it is available.
	AwaitAndReset() (result T, valid bool)

	// Resolves the task result.
	// success is false if the task was already
	// resolved or cancelled.
	Resolve(result T) (success bool)

	// Cancels the task.
	// success is false if the task was already
	// resolved or cancelled.
	Cancel() (success bool)

	// Cancels the task, then sets the error.
	// The error can be retrieved with Error()
	// success is false if the task was already
	// resolved or cancelled.
	Fail(error) (success bool)

	// Returns the error set by Fail().
	// returns nil if there is none.
	Error() error

	// Returns true if Resolve(), Cancel() or Fail() is called.
	IsDone() (done bool)
}

// A unit task represents tasks that doesn't
// return any result.
type UnitTask = Task[Unit]

type taskImpl[T any] struct {
	value        T
	defaultValue T
	status       taskStatus

	awaitMu   sync.Mutex
	resolveMu sync.Mutex

	err error
}

// Regular functions that returns (T, bool)
// are also Awaitable.
type AwaitableFn[T any] func() (T, bool)

func (fn AwaitableFn[T]) Await() (T, bool) {
	return fn()
}

// Creates a new task
func NewTask[T any]() Task[T] {
	t := &taskImpl[T]{}
	t.awaitMu.Lock()
	return t
}

// Creates a new void task
func NewUnitTask() UnitTask {
	t := &taskImpl[Unit]{}
	t.awaitMu.Lock()
	return t
}

// Start the function fn, and returns as task.
// The task is Resolve() when fn returns.
func Start[T any](fn func() T) Task[T] {
	task := NewTask[T]()
	go func() {
		task.Resolve(fn())
	}()
	return task
}

func (task *taskImpl[T]) Resolve(value T) bool {
	task.resolveMu.Lock()
	defer task.resolveMu.Unlock()

	if task.status != taskPending {
		return false
	}

	task.value = value
	task.status = taskResolved
	task.awaitMu.Unlock()
	return true
}

func (task *taskImpl[T]) Error() error {
	return task.err
}

func (task *taskImpl[T]) Fail(err error) bool {
	if task.Cancel() {
		task.err = err
		return true
	}
	return false
}

func (task *taskImpl[T]) Cancel() bool {
	task.resolveMu.Lock()
	defer task.resolveMu.Unlock()

	if task.status == taskPending {
		task.status = taskCanceled
		task.awaitMu.Unlock()
		return true
	}
	return false
}

func (task *taskImpl[T]) IsDone() bool {
	return task.status != taskPending
}

func (task *taskImpl[T]) Await() (T, bool) {
	if task.status == taskPending {
		task.awaitMu.Lock()
		//lint:ignore SA2001 Donkeys
		task.awaitMu.Unlock()
	}
	return task.value, task.status == taskResolved
}

func (task *taskImpl[T]) AwaitAndReset() (T, bool) {
	value, ok := task.Await()
	task.Reset()
	return value, ok
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
	return true
}

// Waits for all tasks or awaitables to finish.
// Returns nil for tasks that have been cancelled.
// The tasks can have different result types.
// Blocks until all tasks are resolved or cancelled.
// Check for nils before derefencing the pointers.
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
func AwaitAll[T any](tasks ...Awaitable[T]) {
	for _, t := range tasks {
		t.Await()
	}
}

// Waits for one task to complete.
// It blocks until at least one task has
// been Resolved() or Cancel().
func AwaitSome[T any](tasks ...Awaitable[T]) {
	blocker := defaultTaskPool.Alloc()
	defer defaultTaskPool.Free(blocker)

	for _, t := range tasks {
		if blocker.IsDone() {
			break
		}
		go func(t Awaitable[T]) {
			t.Await()
			blocker.Resolve(1)
		}(t)
	}

	blocker.Await()
}
