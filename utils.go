package quest

func asPointer[T any](value T, ok bool) *T {
	if !ok {
		return nil
	}
	return &value
}
