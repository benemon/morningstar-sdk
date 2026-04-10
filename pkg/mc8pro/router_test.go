package mc8pro

import (
	"reflect"
	"testing"
	"time"

	"github.com/benemon/morningstar-sdk/pkg/mc8pro/sysex"
)

// TestRouterDispatchMatchingFrame verifies the happy path: a waiter
// is registered, a matching frame arrives, the waiter is woken with
// the frame.
func TestRouterDispatchMatchingFrame(t *testing.T) {
	r := newRouter(nil)
	ch, cancel := r.expect(0x00, 0x7D)
	defer cancel()

	want := sysex.Frame{Cmd1: 0x00, Cmd2: 0x7D, Args: [4]byte{0x01}}
	if !r.dispatch(want) {
		t.Fatal("dispatch returned false; expected true")
	}

	select {
	case got := <-ch:
		if !reflect.DeepEqual(got, want) {
			t.Errorf("received frame %+v, want %+v", got, want)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for dispatched frame")
	}
}

// TestRouterDispatchUnmatchedFrameReturnsFalse verifies that frames
// without registered waiters are silently ignored (the dispatch
// function returns false; the caller logs and moves on).
func TestRouterDispatchUnmatchedFrameReturnsFalse(t *testing.T) {
	r := newRouter(nil)
	frame := sysex.Frame{Cmd1: 0x06, Cmd2: 0x01}
	if r.dispatch(frame) {
		t.Error("dispatch returned true; expected false (no waiter registered)")
	}
}

// TestRouterCancelRemovesWaiter verifies that a cancelled waiter no
// longer claims subsequent frames. After cancel, the next dispatch
// should return false.
func TestRouterCancelRemovesWaiter(t *testing.T) {
	r := newRouter(nil)
	_, cancel := r.expect(0x00, 0x7D)
	cancel()

	frame := sysex.Frame{Cmd1: 0x00, Cmd2: 0x7D}
	if r.dispatch(frame) {
		t.Error("dispatch returned true after cancel; expected false")
	}
}

// TestRouterBroadcastToMultipleSubscribers verifies that when two
// subscribers register filters that both match an incoming frame,
// both receive their own copy in the order frames arrive. This is
// the broadcast semantics of the unified router — distinct from the
// old FIFO-one-shot behavior where each subscriber would have
// received exactly one frame.
func TestRouterBroadcastToMultipleSubscribers(t *testing.T) {
	r := newRouter(nil)
	ch1, cancel1 := r.expect(0x00, 0x7D)
	defer cancel1()
	ch2, cancel2 := r.expect(0x00, 0x7D)
	defer cancel2()

	first := sysex.Frame{Cmd1: 0x00, Cmd2: 0x7D, Args: [4]byte{0x01}}
	second := sysex.Frame{Cmd1: 0x00, Cmd2: 0x7D, Args: [4]byte{0x02}}

	if !r.dispatch(first) {
		t.Fatal("first dispatch returned false")
	}
	if !r.dispatch(second) {
		t.Fatal("second dispatch returned false")
	}

	// Both subscribers should see both frames in order.
	for _, ch := range []<-chan sysex.Frame{ch1, ch2} {
		for _, want := range []sysex.Frame{first, second} {
			select {
			case got := <-ch:
				if !reflect.DeepEqual(got, want) {
					t.Errorf("channel received %+v, want %+v", got, want)
				}
			case <-time.After(100 * time.Millisecond):
				t.Fatalf("timeout waiting for frame %+v", want)
			}
		}
	}
}

// TestRouterNilFilterMatchesEverything verifies that subscribing
// with a nil filter (the dump-collector pattern) receives every
// frame regardless of its command bytes.
func TestRouterNilFilterMatchesEverything(t *testing.T) {
	r := newRouter(nil)
	ch, cancel := r.subscribe(nil)
	defer cancel()

	frames := []sysex.Frame{
		{Cmd1: 0x00, Cmd2: 0x7D},
		{Cmd1: 0x06, Cmd2: 0x01},
		{Cmd1: 0x11, Cmd2: 0x03},
	}
	for _, f := range frames {
		if !r.dispatch(f) {
			t.Errorf("dispatch of %02X %02X returned false", f.Cmd1, f.Cmd2)
		}
	}

	for _, want := range frames {
		select {
		case got := <-ch:
			if !reflect.DeepEqual(got, want) {
				t.Errorf("received %+v, want %+v", got, want)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("timeout waiting for frame %+v", want)
		}
	}
}

// TestRouterFilterIsolation verifies that a filter-matching
// subscriber only receives matching frames, while a nil-filter
// subscriber receives everything. The two should coexist without
// interference.
func TestRouterFilterIsolation(t *testing.T) {
	r := newRouter(nil)
	specific, cancelSpecific := r.expect(0x11, 0x03)
	defer cancelSpecific()
	all, cancelAll := r.subscribe(nil)
	defer cancelAll()

	match := sysex.Frame{Cmd1: 0x11, Cmd2: 0x03}
	nonmatch := sysex.Frame{Cmd1: 0x06, Cmd2: 0x01}

	r.dispatch(match)
	r.dispatch(nonmatch)

	// specific should get only the match
	select {
	case got := <-specific:
		if got.Cmd1 != 0x11 || got.Cmd2 != 0x03 {
			t.Errorf("specific got unexpected frame %+v", got)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("specific subscriber got no frame")
	}
	select {
	case got := <-specific:
		t.Errorf("specific got unexpected second frame %+v", got)
	case <-time.After(50 * time.Millisecond):
		// expected: no second frame
	}

	// all should get both
	for _, want := range []sysex.Frame{match, nonmatch} {
		select {
		case got := <-all:
			if !reflect.DeepEqual(got, want) {
				t.Errorf("all got %+v, want %+v", got, want)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("all subscriber missing frame %+v", want)
		}
	}
}

// TestRouterCloseAllWakesWaiters verifies that closeAll closes all
// pending waiter channels so blocked goroutines unblock during
// Client.Close.
func TestRouterCloseAllWakesWaiters(t *testing.T) {
	r := newRouter(nil)
	ch, cancel := r.expect(0x00, 0x7D)
	defer cancel()

	r.closeAll()

	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to be closed, but received a value")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout: closeAll did not unblock waiter")
	}
}

// TestRouterDifferentKeysDoNotInterfere verifies that a waiter for
// one (cmd1, cmd2) is not woken by a frame with a different key.
func TestRouterDifferentKeysDoNotInterfere(t *testing.T) {
	r := newRouter(nil)
	pingCh, pingCancel := r.expect(0x00, 0x7D)
	defer pingCancel()
	fwCh, fwCancel := r.expect(0x11, 0x03)
	defer fwCancel()

	r.dispatch(sysex.Frame{Cmd1: 0x11, Cmd2: 0x03})

	select {
	case <-fwCh:
		// Expected: firmware waiter woken.
	case <-time.After(100 * time.Millisecond):
		t.Error("firmware waiter not woken by matching frame")
	}

	// Ping waiter should NOT have been woken.
	select {
	case <-pingCh:
		t.Error("ping waiter woken by firmware frame")
	case <-time.After(50 * time.Millisecond):
		// Expected.
	}
}
