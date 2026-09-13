package ws

import (
	"context"
	"encoding/json"
	"os"

	"github.com/redis/go-redis/v9"
)

type RedisBroadcaster struct {
	client *redis.Client
	hub    *Hub
	ctx    context.Context
	cancel context.CancelFunc
}

func NewRedisBroadcaster(hub *Hub) *RedisBroadcaster {
	url := os.Getenv("REDIS_URL")
	if url == "" {
		url = os.Getenv("REDIS_ADDR")
	}
	if url == "" {
		return nil
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		opt = &redis.Options{Addr: url}
	}
	client := redis.NewClient(opt)
	ctx, cancel := context.WithCancel(context.Background())
	// Test connection, if fails return nil but don't crash
	if err := client.Ping(ctx).Err(); err != nil {
		// Redis not available, fallback to single-instance
		cancel()
		return nil
	}
	rb := &RedisBroadcaster{client: client, hub: hub, ctx: ctx, cancel: cancel}
	go rb.subscribe()
	return rb
}

func (r *RedisBroadcaster) PublishRaw(room string, ev Event) {
	if r == nil || r.client == nil {
		return
	}
	data, _ := json.Marshal(ev)
	// Publish to the room as channel (org:xxx or channel:xxx)
	_ = r.client.Publish(r.ctx, room, string(data)).Err()
}

func (r *RedisBroadcaster) subscribe() {
	if r == nil || r.client == nil {
		return
	}
	// Subscribe to all org and channel rooms
	pubsub := r.client.PSubscribe(r.ctx, "org:*", "channel:*")
	defer pubsub.Close()
	ch := pubsub.Channel()
	for {
		select {
		case <-r.ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var ev Event
			if err := json.Unmarshal([]byte(msg.Payload), &ev); err == nil {
				// Use channel as Room if not set (since Room is json:-)
				if ev.Room == "" {
					ev.Room = msg.Channel
				}
				// Deduplicate and broadcast locally without re-publishing to Redis
				r.hub.BroadcastLocal(ev)
			}
		}
	}
}

func (r *RedisBroadcaster) Close() {
	if r != nil && r.cancel != nil {
		r.cancel()
		if r.client != nil {
			_ = r.client.Close()
		}
	}
}
