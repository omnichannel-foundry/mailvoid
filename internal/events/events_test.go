package events

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBusEmitBroadcastsAndRecordsHistory(t *testing.T) {
	var sunk []Event
	bus := NewBus(SinkFunc(func(e Event) error {
		sunk = append(sunk, e)
		return nil
	}))

	ch, unsubscribe := bus.Subscribe(4)
	defer unsubscribe()

	got := bus.Emit(New(TypeSMTPConnected, SeverityInfo, map[string]any{"remote_ip": "127.0.0.1"}))

	if got.ID == "" {
		t.Fatal("expected generated id")
	}
	if got.Timestamp.IsZero() {
		t.Fatal("expected timestamp")
	}
	select {
	case live := <-ch:
		if live.ID != got.ID {
			t.Fatalf("live id = %q, want %q", live.ID, got.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for live event")
	}
	if len(sunk) != 1 || sunk[0].ID != got.ID {
		t.Fatalf("sink did not receive event: %+v", sunk)
	}
	recent := bus.Recent(1)
	if len(recent) != 1 || recent[0].ID != got.ID {
		t.Fatalf("recent = %+v", recent)
	}
}

func TestBusRecentIsBounded(t *testing.T) {
	bus := NewBus()
	for i := 0; i < bus.ringSize+10; i++ {
		bus.Emit(New("test.event", SeverityDebug, nil))
	}
	recent := bus.Recent(0)
	if len(recent) != bus.ringSize {
		t.Fatalf("len(recent) = %d, want %d", len(recent), bus.ringSize)
	}
}

func TestFileSinkWritesJSONLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	sink, err := NewFileSink(path)
	if err != nil {
		t.Fatal(err)
	}

	first := New(TypeMessageReceived, SeverityInfo, map[string]any{"id": "m1"})
	second := New(TypeMessageDropped, SeverityWarn, map[string]any{"id": "m2"})
	if err := sink.Write(first); err != nil {
		t.Fatal(err)
	}
	if err := sink.Write(second); err != nil {
		t.Fatal(err)
	}
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var lines []Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var ev Event
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			t.Fatalf("invalid json line: %v", err)
		}
		lines = append(lines, ev)
	}
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if lines[0].Type != TypeMessageReceived || lines[1].Type != TypeMessageDropped {
		t.Fatalf("unexpected event order: %+v", lines)
	}
}

func TestUnsubscribeClosesChannel(t *testing.T) {
	bus := NewBus()
	ch, unsubscribe := bus.Subscribe(1)
	unsubscribe()
	if _, ok := <-ch; ok {
		t.Fatal("expected closed channel")
	}
}

func TestAddSinkAndDefaults(t *testing.T) {
	var got []Event
	bus := NewBus()
	bus.AddSink(SinkFunc(func(event Event) error {
		got = append(got, event)
		return nil
	}))
	event := bus.Emit(Event{Type: "custom.event", Payload: map[string]any{"n": 1}})
	if event.ID == "" || event.Timestamp.IsZero() || event.Severity != SeverityInfo {
		t.Fatalf("defaults not applied: %+v", event)
	}
	if len(got) != 1 || got[0].ID != event.ID {
		t.Fatalf("sink = %+v", got)
	}
}

func TestSinkErrorsAreIgnored(t *testing.T) {
	bus := NewBus(SinkFunc(func(Event) error { return os.ErrClosed }))
	ch, unsubscribe := bus.Subscribe(1)
	defer unsubscribe()
	event := bus.Emit(New("test.event", SeverityWarn, nil))
	select {
	case live := <-ch:
		if live.ID != event.ID {
			t.Fatalf("live = %+v", live)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive event")
	}
}

func TestSubscribeDefaultBufferAndRecentLimits(t *testing.T) {
	bus := NewBus()
	ch, unsubscribe := bus.Subscribe(0)
	defer unsubscribe()
	if cap(ch) != 32 {
		t.Fatalf("buffer = %d", cap(ch))
	}
	for i := 0; i < 5; i++ {
		bus.Emit(New("test.event", SeverityDebug, nil))
	}
	if len(bus.Recent(-1)) != 5 || len(bus.Recent(99)) != 5 || len(bus.Recent(2)) != 2 {
		t.Fatalf("recent limits failed")
	}
}

func TestFileSinkErrors(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileSink(filepath.Join(blocker, "events.jsonl")); err == nil {
		t.Fatal("expected mkdir error")
	}

	sink, err := NewFileSink(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}
	if err := sink.Write(New("test.event", SeverityInfo, nil)); err != os.ErrClosed {
		t.Fatalf("write after close = %v", err)
	}
}

func TestEventMarshalRoundTrip(t *testing.T) {
	event := New(TypeMessageReceived, SeverityInfo, map[string]any{"id": "m1"})
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Event
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ID != event.ID || decoded.Type != event.Type || decoded.Payload["id"] != "m1" {
		t.Fatalf("decoded = %+v", decoded)
	}
}
