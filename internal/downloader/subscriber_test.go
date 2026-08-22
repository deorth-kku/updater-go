package downloader

import (
	"context"
	"testing"
	"time"

	aria2 "github.com/deorth-kku/aria2rpc-go"
)

// TestSubscribeDeliversLateEvent verifies the fix for the ws-mode hang: when a
// completion notification races ahead of subscribe() (e.g. a fast/resumed
// download finishing before AddURI returns), the event is buffered and delivered
// immediately instead of being dropped, which previously left waitForWS blocked
// forever.
func TestSubscribeDeliversLateEvent(t *testing.T) {
	s := newSubscriber()
	ctx := context.Background()

	// Fire the completion notification before anyone subscribes.
	s.OnDownloadComplete(ctx, aria2.DownloadEvent{GID: "gid-1"})

	ch := s.subscribe("gid-1")
	select {
	case status := <-ch:
		if status != statusComplete {
			t.Fatalf("got status %v, want statusComplete", status)
		}
	case <-time.After(time.Second):
		t.Fatal("subscribe did not receive the buffered completion event (race not fixed)")
	}
}

// TestSubscribeNormalEvent verifies the common path still works: a subscriber
// registered before the notification receives it on the channel.
func TestSubscribeNormalEvent(t *testing.T) {
	s := newSubscriber()
	ctx := context.Background()

	ch := s.subscribe("gid-2")
	s.OnDownloadStop(ctx, aria2.DownloadEvent{GID: "gid-2"})

	select {
	case status := <-ch:
		if status != statusStopped {
			t.Fatalf("got status %v, want statusStopped", status)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive the notification")
	}
}
