package service

import (
	"context"
	"sync"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/proto"
	pb "nonoka-im/api/im/v1"
	"nonoka-im/internal/gateway"
)

const (
	// pushWorkerCount is the number of goroutines used for concurrent push.
	// Tuned for typical IM gateway scenarios: 20 workers balance
	// parallelism with gRPC connection multiplexing overhead.
	pushWorkerCount = 20
)

// PushService is exposed by the Gateway for MsgWorker to push messages to online users.
type PushService struct {
	pb.UnimplementedPushServiceServer
	manager *gateway.Manager
	log     *log.Helper
}

// NewPushService creates a new PushService with the connection manager.
func NewPushService(manager *gateway.Manager, logger log.Logger) *PushService {
	return &PushService{
		manager: manager,
		log:     log.NewHelper(logger),
	}
}

// PushToUser delivers a message to all online devices of a user.
func (s *PushService) PushToUser(ctx context.Context, req *pb.PushToUserRequest) (*pb.PushToUserReply, error) {
	msg := req.GetMessage()
	if msg == nil {
		return &pb.PushToUserReply{Success: false, DeliveredCount: 0}, nil
	}

	packet := &pb.Packet{
		Cmd: pb.Command_CMD_NOTIFY,
		Payload: &pb.Packet_Notify{
			Notify: msg,
		},
	}

	delivered := s.manager.BroadcastToUser(req.GetUserId(), packet)
	s.log.Debugf("push to user: user_id=%d, delivered=%d", req.GetUserId(), delivered)

	return &pb.PushToUserReply{
		Success:        delivered > 0,
		DeliveredCount: int32(delivered),
	}, nil
}

// BatchPushToUsers delivers messages to multiple users concurrently.
// It pre-serializes the packet once and uses a worker pool to parallelize
// delivery across users, significantly reducing latency for large batches.
func (s *PushService) BatchPushToUsers(ctx context.Context, req *pb.BatchPushToUsersRequest) (*pb.BatchPushToUsersReply, error) {
	msg := req.GetMessage()
	if msg == nil {
		return &pb.BatchPushToUsersReply{TotalDelivered: 0}, nil
	}

	userIDs := req.GetUserIds()
	if len(userIDs) == 0 {
		return &pb.BatchPushToUsersReply{TotalDelivered: 0}, nil
	}

	// Pre-serialize the packet once to avoid repeated protobuf marshaling
	packet := &pb.Packet{
		Cmd: pb.Command_CMD_NOTIFY,
		Payload: &pb.Packet_Notify{
			Notify: msg,
		},
	}
	marshaled, err := proto.Marshal(packet)
	if err != nil {
		s.log.Errorf("marshal packet for batch push failed: %v", err)
		return &pb.BatchPushToUsersReply{TotalDelivered: 0}, nil
	}

	// Use worker pool for concurrent delivery
	totalDelivered, failedUserIDs := s.batchPushConcurrent(ctx, userIDs, marshaled)

	s.log.Debugf("batch push: users=%d, total_delivered=%d, failed=%d",
		len(userIDs), totalDelivered, len(failedUserIDs))

	return &pb.BatchPushToUsersReply{
		TotalDelivered: int32(totalDelivered),
		FailedUserIds:  failedUserIDs,
	}, nil
}

// batchPushConcurrent distributes users across a fixed worker pool.
func (s *PushService) batchPushConcurrent(ctx context.Context, userIDs []int64, marshaled []byte) (int, []int64) {
	workerCount := min(len(userIDs), pushWorkerCount)

	type result struct {
		userID    int64
		delivered int
	}

	var wg sync.WaitGroup
	resultCh := make(chan result, len(userIDs))

	// Split users into chunks for each worker
	chunkSize := (len(userIDs) + workerCount - 1) / workerCount

	for i := range workerCount {
		start := i * chunkSize
		end := start + chunkSize
		if start >= len(userIDs) {
			break
		}
		if end > len(userIDs) {
			end = len(userIDs)
		}

		wg.Add(1)
		go func(ids []int64) {
			defer wg.Done()
			for _, userID := range ids {
				// Check context cancellation between users
				select {
				case <-ctx.Done():
					return
				default:
				}

				delivered := s.manager.BroadcastToUserRaw(userID, marshaled)
				resultCh <- result{userID: userID, delivered: delivered}
			}
		}(userIDs[start:end])
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	totalDelivered := 0
	var failedUserIDs []int64
	for r := range resultCh {
		if r.delivered == 0 {
			failedUserIDs = append(failedUserIDs, r.userID)
		}
		totalDelivered += r.delivered
	}

	return totalDelivered, failedUserIDs
}
