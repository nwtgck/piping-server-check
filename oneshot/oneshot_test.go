package oneshot

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestOneshotCached(t *testing.T) {
	oneshot := NewOneshot[string]()
	// not blocked
	oneshot.Send("my message")
	// value got
	{
		value := <-oneshot.Channel()
		assert.Equal(t, "my message", value)
	}
	// value got again
	{
		value := <-oneshot.Channel()
		assert.Equal(t, "my message", value)
	}
	oneshot.Done()
	// value got after done
	{
		value := <-oneshot.Channel()
		assert.Equal(t, "my message", value)
	}
}

func TestDoneWithoutSend(t *testing.T) {
	oneshot := NewOneshot[string]()
	oneshot.Done()
	// no value after done without send
	{
		_, ok := <-oneshot.Channel()
		assert.Equal(t, false, ok)
	}
}

func TestOneshotChannelClosed(t *testing.T) {
	oneshot := NewOneshot[string]()
	// not blocked
	oneshot.Send("my message")
	// channel from .Channel() is closed after value got
	{
		ch := oneshot.Channel()
		var _ = <-ch
		_, ok := <-ch
		assert.Equal(t, false, ok)
	}
}

func TestOneshotSendTwice(t *testing.T) {
	oneshot := NewOneshot[string]()
	oneshot.Send("my message 1")
	assert.Panics(t, func() {
		oneshot.Send("my message 2")
	})
}

func TestOneshotSendAfterDone(t *testing.T) {
	oneshot := NewOneshot[string]()
	oneshot.Done()
	assert.Panics(t, func() {
		oneshot.Send("my message")
	})
}
