package main

import (
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/Waasaabii/meowhomo/engine/proto"
)

type LogHub struct {
	mu                 sync.Mutex
	nextID             int
	subscribers        map[int]chan *pb.LogEntry
	subscriberSnapshot atomic.Value
}

// NewLogHub 创建日志发布中心，用于支持多订阅者广播。
func NewLogHub() *LogHub {
	hub := &LogHub{
		subscribers: make(map[int]chan *pb.LogEntry),
	}
	hub.subscriberSnapshot.Store([]chan *pb.LogEntry{})
	return hub
}

// Subscribe 注册一个日志订阅并返回订阅 ID 与接收通道。
func (hub *LogHub) Subscribe() (int, <-chan *pb.LogEntry) {
	hub.mu.Lock()
	defer hub.mu.Unlock()

	hub.nextID++
	id := hub.nextID
	channel := make(chan *pb.LogEntry, 64)
	hub.subscribers[id] = channel
	hub.rebuildSnapshotLocked()
	return id, channel
}

// Unsubscribe 取消订阅。
//
// 注意：这里不主动 close 订阅通道，避免 Publish 并发发送时触发 send on closed channel。
// StreamLogs 的消费协程会通过 context 退出并释放通道引用，由 GC 回收。
func (hub *LogHub) Unsubscribe(id int) {
	hub.mu.Lock()
	defer hub.mu.Unlock()

	if _, exists := hub.subscribers[id]; !exists {
		return
	}

	delete(hub.subscribers, id)
	hub.rebuildSnapshotLocked()
}

// Publish 向所有订阅者广播一条日志（慢消费者会被丢弃本条消息）。
func (hub *LogHub) Publish(source, level, message string) {
	entry := &pb.LogEntry{
		Source:    source,
		Level:     level,
		Message:   message,
		Timestamp: time.Now().Unix(),
	}

	channels, _ := hub.subscriberSnapshot.Load().([]chan *pb.LogEntry)
	for _, channel := range channels {
		select {
		case channel <- entry:
		default:
		}
	}
}

func (hub *LogHub) rebuildSnapshotLocked() {
	snapshot := make([]chan *pb.LogEntry, 0, len(hub.subscribers))
	for _, channel := range hub.subscribers {
		snapshot = append(snapshot, channel)
	}
	hub.subscriberSnapshot.Store(snapshot)
}
