package quest

import "sync"

// An object pool for tasks. Use only
// when gc is an issue.
type TaskPool[T any] struct {
	syncPool sync.Pool
}

func NewTaskPool[T any](initSize int) *TaskPool[T] {
	pool := &TaskPool[T]{
		syncPool: sync.Pool{
			New: func() any {
				return NewTask[T]()
			},
		},
	}
	for i := 0; i < initSize; i++ {
		pool.Free(NewTask[T]())
	}

	return pool
}

func (taskPool *TaskPool[T]) Alloc() Task[T] {
	item := taskPool.syncPool.Get().(*taskImpl[T])
	item.enable()
	item.Reset()
	return item
}

func (taskPool *TaskPool[T]) Free(item Task[T]) {
	if task, ok := item.(*taskImpl[T]); ok {
		task.disable()
		task.Cancel()
		taskPool.syncPool.Put(task)
	}
}

var defaultTaskPool = NewTaskPool[any](100)
