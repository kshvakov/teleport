package teleport

import (
	"reflect"
)

type handler struct {
	fn   reflect.Value
	args Args
}

func (handler *handler) call(ctx Context, args Args) error {
	result := handler.fn.Call([]reflect.Value{
		reflect.ValueOf(ctx),
		reflect.ValueOf(args),
	})
	if result[0].IsNil() {
		return nil
	}
	return result[0].Interface().(error)
}
