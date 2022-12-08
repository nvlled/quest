# quest

This go library provides async features similar to C#'s Task or Javascript's
async/promise.

## Purpose and motivation

This project is initially written for a [coroutine](#) library,
but it seems to be general enough as a standalone library itself.
Go channels were previously considered, but channels are

- prone to deadlocks
- not reusable after closing
- opaque, it's state cannot be inspected
- too low-level for async code

This is not say that channels are useless, but channels
are inadequate in the context game coroutines,
where complex state machines are executed in different frames.
Which is not to say that channels can't be used, but
doing so would require a lot of clunky boilerplate to handle
the points listed above.

Why not just use callbacks and higher-order functions?
Same reasons, highly asynchronous code (i.e. game scripts)
results to clunky callback hell when plain callbacks are used.

## Quick examples

```go
task1 := quest.NewTask[float32]
task2 := quest.NewTask[int]
task3 := quest.Start(func() Unit {
  // do something
})

go func() {
  task1.Cancel()
  task2.Resolve(0)
  task3.Fail(errors.New("nope"))
}()

result, ok := task1.Await()
x, y := quest.Await2(task2, task3)
```

```go
func doSomething(id string) Task[int] {/* omitted */ }
func doOtherSomething(id string) Task[string] {/* omitted */ }
func getSomething(id string) any {
  a, ok := doSomething(id).Await()
  if !ok { return }
  b, ok := doOtherThing(id).Await()
  if !ok { return }
  return doTheThing(a, b)
}
```

Or alternatively:

```go
func getSomething(id string) any {
  a, b := quest.Wait2(doSomething(id), doOtherThing(id))
  if a == nil || b == nil { return }
  return doTheThing(*a, *b)
}
```

For more examples, see test files.
