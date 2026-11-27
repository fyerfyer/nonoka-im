package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"
	v1 "nonoka-im/api/im/v1"
)

const peerFanoutChannel = "im:gateway:fanout"

// peerFanout is the internal wire shape for gateway-to-gateway fan-out.
// The packet is serialized once by the publisher and broadcast locally by
// each receiving gateway.
type peerFanout struct {
	UserIDs []int64 `json:"user_ids"`
	Data    []byte  `json:"data"`
}

// PeerBus fans out receipt/read/recall packets to every gateway node. User
// sessions are distributed across gateway processes, so a receipt generated
// by the gateway that received an ACK may belong to a user connected to
// another node.
type PeerBus struct {
	redis   redis.UniversalClient
	manager *Manager
	log     *log.Helper

	mu       sync.Mutex
	pubsub   *redis.PubSub
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

func NewPeerBus(redisClient redis.UniversalClient, manager *Manager, logger log.Logger) *PeerBus {
	return &PeerBus{
		redis: redisClient, manager: manager,
		log: log.NewHelper(logger), stopCh: make(chan struct{}),
	}
}

// Start subscribes the gateway to cross-node packets. It is safe to call more
// than once; only the first call creates a subscription.
func (b *PeerBus) Start(ctx context.Context) error {
	if b == nil || b.redis == nil {
		return nil
	}
	b.mu.Lock()
	if b.pubsub != nil {
		b.mu.Unlock()
		return nil
	}
	b.pubsub = b.redis.Subscribe(ctx, peerFanoutChannel)
	b.mu.Unlock()
	b.wg.Add(1)
	go b.run(ctx)
	return nil
}

func (b *PeerBus) run(ctx context.Context) {
	defer b.wg.Done()
	ch := b.pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case <-b.stopCh:
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var event peerFanout
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
				b.log.Warnf("decode gateway peer fanout: %v", err)
				continue
			}
			seen := make(map[int64]struct{}, len(event.UserIDs))
			for _, userID := range event.UserIDs {
				if userID <= 0 {
					continue
				}
				if _, ok := seen[userID]; ok {
					continue
				}
				seen[userID] = struct{}{}
				b.manager.BroadcastToUserRaw(userID, event.Data)
			}
		}
	}
}

// Publish sends a packet to all gateway nodes. A caller that gets an error
// must fall back to its local manager; Redis is best-effort for this path.
func (b *PeerBus) Publish(ctx context.Context, userIDs []int64, packet *v1.Packet) error {
	if b == nil || b.redis == nil {
		return fmt.Errorf("gateway peer bus is not configured")
	}
	if len(userIDs) == 0 || packet == nil {
		return nil
	}
	data, err := proto.Marshal(packet)
	if err != nil {
		return fmt.Errorf("marshal peer fanout packet: %w", err)
	}
	payload, err := json.Marshal(peerFanout{UserIDs: userIDs, Data: data})
	if err != nil {
		return fmt.Errorf("marshal peer fanout envelope: %w", err)
	}
	pubCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := b.redis.Publish(pubCtx, peerFanoutChannel, payload).Err(); err != nil {
		return fmt.Errorf("publish gateway peer fanout: %w", err)
	}
	return nil
}

func (b *PeerBus) Stop() {
	if b == nil {
		return
	}
	b.stopOnce.Do(func() { close(b.stopCh) })
	b.mu.Lock()
	pubsub := b.pubsub
	b.mu.Unlock()
	if pubsub != nil {
		_ = pubsub.Close()
	}
	b.wg.Wait()
}
