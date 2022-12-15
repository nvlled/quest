package quest

import (
	"github.com/nvlled/mud"
)

func init() {
	PreAllocTasks[Void](100)
}

// Pre-allocate a number of tasks of the given type.
func PreAllocTasks[T any](numTasks int) {
	mud.PreAlloc(taskPool, newTask[T], numTasks)
}

// Allocate a task using an object pool.
// Free the task afterwards with Free().
// Use only when gc is a concern.
func AllocTask[T any]() Task[T] {
	task := mud.Alloc(taskPool, newTask[T])
	task.disabled = false
	task.Reset()
	return task
}

// Free a task that was previously Alloc()'d.
func FreeTask[T any](task Task[T]) {
	object, ok := task.(*taskImpl[T])
	if !ok {
		return
	}
	mud.Free(taskPool, object)
}

var taskPool = mud.NewPool()
