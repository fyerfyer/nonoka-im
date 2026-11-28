package realtime

import (
	"context"
	"io"
	"testing"

	pb "nonoka-im/api/im/v1"

	"github.com/go-kratos/kratos/v2/log"
)

type offlineRouter struct{}

func (offlineRouter) ResolveUserNodes(context.Context, []int64) (map[string][]int64, error) {
	return map[string][]int64{}, nil
}
func (offlineRouter) ResolveUserNodesWithNodes(context.Context, []int64, []*GatewayNodeInfo) (map[string][]int64, error) {
	return map[string][]int64{}, nil
}
func (offlineRouter) GetAliveNodes(context.Context) ([]*GatewayNodeInfo, error) {
	return nil, nil
}
func (offlineRouter) GetAliveNodesMap(context.Context) (map[string]*GatewayNodeInfo, error) {
	return map[string]*GatewayNodeInfo{}, nil
}

// Offline users are durable-pull clients, not transient failures. Reporting
// them as failed made MsgWorker enqueue a Redis push retry (with the full
// message body) for every offline group member.
func TestBatchPushWithRoutingDoesNotReportOfflineUsersAsFailed(t *testing.T) {
	pusher, err := NewGatewayPusher([]string{"127.0.0.1:1"}, log.NewStdLogger(io.Discard))
	if err != nil {
		t.Fatal(err)
	}
	defer pusher.Close()
	pusher.SetRouter(offlineRouter{})

	delivered, failed, err := pusher.BatchPushToUsers(context.Background(), []int64{1, 2}, &pb.MessagePush{MsgId: 1, Topic: "grp_x"})
	if err != nil {
		t.Fatalf("batch push: %v", err)
	}
	if delivered != 0 {
		t.Fatalf("delivered = %d, want 0", delivered)
	}
	if len(failed) != 0 {
		t.Fatalf("offline users reported as failed: %v", failed)
	}
}
