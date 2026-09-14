package main

import "testing"

// TestReconnectHookCallBeforeSetIsNoOp confirms a reconnect firing before main() has finished
// building its dependencies (the state every connectMQTT client's OnConnectHandler is in during
// the very first connect) never panics -- it's a safe no-op until Set has run.
func TestReconnectHookCallBeforeSetIsNoOp(t *testing.T) {
	hook := &TReconnectHook{}
	hook.call() // must not panic
}

// TestReconnectHookCallInvokesSetFunction confirms the real, expected shape: once main() has set
// a real republish function, every subsequent call() (one per reconnect) actually runs it, every
// time, not just once.
func TestReconnectHookCallInvokesSetFunction(t *testing.T) {
	hook := &TReconnectHook{}
	calls := 0
	hook.Set(func() { calls++ })

	hook.call()
	hook.call()
	hook.call()

	if calls != 3 {
		t.Errorf("calls = %d, want 3 (call() must invoke the set function every time, not just once)", calls)
	}
}

// TestReconnectHookSetReplacesPreviousFunction confirms a later Set (not expected in the
// coordinator's own real usage today, which sets it exactly once, but worth locking in) replaces
// rather than accumulates.
func TestReconnectHookSetReplacesPreviousFunction(t *testing.T) {
	hook := &TReconnectHook{}
	firstCalls, secondCalls := 0, 0
	hook.Set(func() { firstCalls++ })
	hook.Set(func() { secondCalls++ })

	hook.call()

	if firstCalls != 0 {
		t.Errorf("firstCalls = %d, want 0 (Set must replace, not add to, the previous function)", firstCalls)
	}
	if secondCalls != 1 {
		t.Errorf("secondCalls = %d, want 1", secondCalls)
	}
}
