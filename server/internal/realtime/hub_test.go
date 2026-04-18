package realtime

import "testing"

func TestHub_SubscribeUnsubscribe(t *testing.T) {
	h := NewHub()
	h.Subscribe("u1")
	h.Subscribe("u1")
	h.Unsubscribe("u1") // count should be 1
	// no panic, still tracked
	h.Unsubscribe("u1") // removes
	h.Unsubscribe("u1") // no-op on missing key
}

func TestHub_PublishAndLastEvent(t *testing.T) {
	h := NewHub()

	_, ok := h.LastEvent("u1")
	if ok {
		t.Error("expected no event before publish")
	}

	evt := Event{UserID: "u1", Type: "test", Payload: map[string]any{"k": "v"}}
	h.Publish(evt)

	got, ok := h.LastEvent("u1")
	if !ok {
		t.Fatal("expected event after publish")
	}
	if got.Type != "test" {
		t.Errorf("Type = %q, want %q", got.Type, "test")
	}
	if got.Payload["k"] != "v" {
		t.Errorf("Payload[k] = %v", got.Payload["k"])
	}

	// overwrite
	h.Publish(Event{UserID: "u1", Type: "second"})
	got, _ = h.LastEvent("u1")
	if got.Type != "second" {
		t.Errorf("expected overwritten event, got %q", got.Type)
	}

	// different user
	_, ok = h.LastEvent("u2")
	if ok {
		t.Error("expected no event for u2")
	}
}
