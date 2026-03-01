package main

import (
	"sync"
	"testing"
	"time"
)

func TestLogHubPublishAndUnsubscribe(t *testing.T) {
	hub := NewLogHub()
	subscriptionID, channel := hub.Subscribe()

	hub.Publish("内核喵", "info", "engine started")

	select {
	case entry := <-channel:
		if entry.GetSource() != "内核喵" {
			t.Fatalf("unexpected source: %s", entry.GetSource())
		}
		if entry.GetLevel() != "info" {
			t.Fatalf("unexpected level: %s", entry.GetLevel())
		}
		if entry.GetMessage() != "engine started" {
			t.Fatalf("unexpected message: %s", entry.GetMessage())
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("did not receive published log entry")
	}

	hub.Unsubscribe(subscriptionID)

	hub.Publish("内核喵", "info", "should not be delivered")

	select {
	case entry := <-channel:
		t.Fatalf("unexpected log after unsubscribe: %+v", entry)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestLogHubConcurrentPublishAndUnsubscribe(t *testing.T) {
	hub := NewLogHub()
	subscriptionID, _ := hub.Subscribe()

	var waitGroup sync.WaitGroup
	for index := 0; index < 8; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for count := 0; count < 1000; count++ {
				hub.Publish("内核喵", "info", "burst")
			}
		}()
	}

	hub.Unsubscribe(subscriptionID)
	waitGroup.Wait()
}
