package oneshot

import (
	"sync"
)

// Oneshot is oneshot cached channel
type Oneshot[T any] struct {
	ch     chan T
	done   chan struct{}
	cached bool
	value  T
	wg     *sync.WaitGroup
}

// NewOneshot makes new Oneshot
func NewOneshot[T any]() *Oneshot[T] {
	ch := make(chan T)
	done := make(chan struct{})
	wg := new(sync.WaitGroup)
	oneshot := &Oneshot[T]{ch: ch, done: done, wg: wg}
	wg.Add(1)
	go func() {
		select {
		case v := <-ch:
			oneshot.value = v
			oneshot.cached = true
			wg.Done()
		case <-done:
			wg.Done()
		}
	}()
	return oneshot
}

// Send sends value without blocking
func (o *Oneshot[T]) Send(value T) {
	o.ch <- value
}

// Channel returns a channel to get value. The value is always returned when already Send() called no matter Done() called.
// Channel returns a closed channel after Done() called without no Send() call.
func (o *Oneshot[T]) Channel() <-chan T {
	ch := make(chan T, 1)
	if o.cached {
		ch <- o.value
		close(ch)
		return ch
	}
	select {
	case <-o.done:
		close(ch)
		return ch
	default:
	}
	go func() {
		o.wg.Wait()
		if o.cached {
			ch <- o.value
		}
		close(ch)
	}()
	return ch
}

// Done finishes a channel from Channel()
func (o *Oneshot[T]) Done() {
	close(o.done)
}
