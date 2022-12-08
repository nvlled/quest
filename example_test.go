package quest_test

import (
	"fmt"

	"github.com/nvlled/quest"
)

func ExampleTask_Resolve() {
	task1 := quest.NewTask[int]()
	task2 := quest.NewTask[int]()

	go func() {
		// ... do some computation, then
		task1.Resolve(1000)
		task2.Resolve(2000)

		// resolving again won't have any effect
		task1.Resolve(3000) // doesn't work

		// ... unless the task is reset first
		task1.Reset()
		task1.Resolve(3000) // now works
	}()

	result1, _ := task1.Await()
	result2, _ := task2.Await()

	// await another result
	result1, _ = task1.Await()

	fmt.Printf("task1=%v, task2=%v\n", result1, result2)
	// Output: task1=3000, task2=2000
}
