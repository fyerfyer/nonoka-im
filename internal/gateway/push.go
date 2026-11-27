package gateway

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/proto"
	pb "nonoka-im/api/im/v1"
)

const (
	// pushWorkerCount is the number of goroutines used for concurrent push.
	// It defaults to 2 * CPU cores, capped between 4 and 64, to adapt
	// to different hardware while avoiding excessive gRPC overhead (#21).
	pushWorkerCountMin = 4
	pushWorkerCountMax = 64
)

// getPushWorkerCount returns a dynamic worker count based on CPU cores.
func getPushWorkerCount() int {
	n := runtime.NumCPU() * 2
	if n < pushWorkerCountMin {
		return pushWorkerCountMin
	}
	if n > pushWorkerCountMax {
		return pushWorkerCountMax
	}
	return n
}

// PushService is exposed by the Gateway for MsgWorker to push messages to online users.
type PushService struct {
	pb.UnimplementedPushServiceServer
	manager *Manager
	log     *log.Helper
}

// NewPushService creates a new PushService with the connection manager.
func NewPushService(manager *Manager, logger log.Logger) *PushService {
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

// PushRunEventToUser delivers one transient agent run event to all online
// devices of a user.
func (s *PushService) PushRunEventToUser(ctx context.Context, req *pb.PushRunEventToUserRequest) (*pb.PushRunEventToUserReply, error) {
	event := req.GetEvent()
	if event == nil {
		return &pb.PushRunEventToUserReply{Success: false}, nil
	}
	packet := &pb.Packet{
		Cmd: pb.Command_CMD_AGENT_RUN_EVENT,
		Payload: &pb.Packet_AgentRunEvent{
			AgentRunEvent: event,
		},
	}
	delivered := s.manager.BroadcastToUser(req.GetUserId(), packet)
	s.log.Debugf("push run event: user_id=%d run_id=%d type=%s delivered=%d", req.GetUserId(), event.GetRunId(), event.GetType(), delivered)
	return &pb.PushRunEventToUserReply{Success: delivered > 0, DeliveredCount: int32(delivered)}, nil
}

// PushBotStreamEventToUser delivers one transient external-bot reply event to
// all online devices of a user.
func (s *PushService) PushBotStreamEventToUser(ctx context.Context, req *pb.PushBotStreamEventToUserRequest) (*pb.PushBotStreamEventToUserReply, error) {
	event := req.GetEvent()
	if event == nil {
		return &pb.PushBotStreamEventToUserReply{Success: false}, nil
	}
	packet := &pb.Packet{
		Cmd: pb.Command_CMD_BOT_STREAM_EVENT,
		Payload: &pb.Packet_BotStreamEvent{
			BotStreamEvent: event,
		},
	}
	delivered := s.manager.BroadcastToUser(req.GetUserId(), packet)
	s.log.Debugf("push bot stream event: user_id=%d stream_id=%s type=%s delivered=%d",
		req.GetUserId(), event.GetStreamId(), event.GetType(), delivered)
	return &pb.PushBotStreamEventToUserReply{Success: delivered > 0, DeliveredCount: int32(delivered)}, nil
}

// PushReceiptToUser delivers a send receipt to all online devices of a user.
func (s *PushService) PushReceiptToUser(ctx context.Context, req *pb.PushReceiptToUserRequest) (*pb.PushReceiptToUserReply, error) {
	receipt := req.GetReceipt()
	if receipt == nil {
		return &pb.PushReceiptToUserReply{Success: false, DeliveredCount: 0}, nil
	}

	packet := &pb.Packet{
		Cmd: pb.Command_CMD_SEND_RECEIPT,
		Payload: &pb.Packet_SendReceipt{
			SendReceipt: receipt,
		},
	}

	delivered := s.manager.BroadcastToUser(req.GetUserId(), packet)
	s.log.Debugf("push receipt to user: user_id=%d, delivered=%d", req.GetUserId(), delivered)

	return &pb.PushReceiptToUserReply{
		Success:        delivered > 0,
		DeliveredCount: int32(delivered),
	}, nil
}

// BatchPushReceiptToUsers delivers send receipts to multiple users concurrently.
func (s *PushService) BatchPushReceiptToUsers(ctx context.Context, req *pb.BatchPushReceiptToUsersRequest) (*pb.BatchPushReceiptToUsersReply, error) {
	receipt := req.GetReceipt()
	if receipt == nil {
		return &pb.BatchPushReceiptToUsersReply{TotalDelivered: 0}, nil
	}

	userIDs := req.GetUserIds()
	if len(userIDs) == 0 {
		return &pb.BatchPushReceiptToUsersReply{TotalDelivered: 0}, nil
	}

	// Pre-serialize the packet once to avoid repeated protobuf marshaling
	packet := &pb.Packet{
		Cmd: pb.Command_CMD_SEND_RECEIPT,
		Payload: &pb.Packet_SendReceipt{
			SendReceipt: receipt,
		},
	}
	marshaled, err := proto.Marshal(packet)
	if err != nil {
		s.log.Errorf("marshal receipt packet for batch push failed: %v", err)
		return &pb.BatchPushReceiptToUsersReply{TotalDelivered: 0}, nil
	}

	// Use worker pool for concurrent delivery
	totalDelivered, failedUserIDs := s.batchPushConcurrent(ctx, userIDs, marshaled)

	s.log.Debugf("batch push receipts: users=%d, total_delivered=%d, failed=%d",
		len(userIDs), totalDelivered, len(failedUserIDs))

	return &pb.BatchPushReceiptToUsersReply{
		TotalDelivered: int32(totalDelivered),
		FailedUserIds:  failedUserIDs,
	}, nil
}

// BatchPushReceiptsToUsers delivers multiple distinct send receipts to multiple
// users in one call. Receipts are grouped by user and pushed together to reduce
// gRPC round-trips.
func (s *PushService) BatchPushReceiptsToUsers(ctx context.Context, req *pb.BatchPushReceiptsToUsersRequest) (*pb.BatchPushReceiptsToUsersReply, error) {
	items := req.GetItems()
	if len(items) == 0 {
		return &pb.BatchPushReceiptsToUsersReply{TotalDelivered: 0}, nil
	}

	// Group receipts by user so each user gets one broadcast with all receipts.
	userItems := make(map[int64][]*pb.ReceiptBatchItem)
	userReceipts := make(map[int64][]*pb.SendReceipt)
	for _, item := range items {
		if item.GetReceipt() == nil {
			continue
		}
		uid := item.GetUserId()
		userItems[uid] = append(userItems[uid], item)
		userReceipts[uid] = append(userReceipts[uid], item.GetReceipt())
	}

	type result struct {
		userID    int64
		delivered int
	}

	workerCount := min(len(userReceipts), getPushWorkerCount())
	if workerCount < 1 {
		workerCount = 1
	}
	jobs := make(chan int64)
	resultCh := make(chan result, len(userReceipts))
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for userID := range jobs {
				delivered := s.broadcastReceiptsToUser(userID, userReceipts[userID])
				resultCh <- result{userID: userID, delivered: delivered}
			}
		}()
	}
	for uid := range userReceipts {
		jobs <- uid
	}
	close(jobs)
	wg.Wait()
	close(resultCh)

	totalDelivered := 0
	failedItems := make([]*pb.ReceiptBatchItem, 0)
	for r := range resultCh {
		totalDelivered += r.delivered
		if r.delivered == 0 {
			failedItems = append(failedItems, userItems[r.userID]...)
		}
	}

	s.log.Debugf("batch push receipts (plural): items=%d users=%d total_delivered=%d failed=%d",
		len(items), len(userReceipts), totalDelivered, len(failedItems))

	return &pb.BatchPushReceiptsToUsersReply{
		TotalDelivered: int32(totalDelivered),
		FailedItems:    failedItems,
	}, nil
}

// broadcastReceiptsToUser pushes one or more send receipts to all devices of a
// user. It pre-serializes each receipt once and reuses the bytes across devices.
func (s *PushService) broadcastReceiptsToUser(userID int64, receipts []*pb.SendReceipt) int {
	conns := s.manager.GetAll(userID)
	if len(conns) == 0 {
		return 0
	}

	var data [][]byte
	for _, receipt := range receipts {
		packet := &pb.Packet{
			Cmd: pb.Command_CMD_SEND_RECEIPT,
			Payload: &pb.Packet_SendReceipt{
				SendReceipt: receipt,
			},
		}
		b, err := proto.Marshal(packet)
		if err != nil {
			s.log.Warnf("marshal receipt for user %d failed: %v", userID, err)
			continue
		}
		data = append(data, b)
	}
	if len(data) == 0 {
		return 0
	}

	sent := 0
	for _, c := range conns {
		for _, b := range data {
			if err := c.SendRawBytesWithTimeoutUnsafe(b, 100*time.Millisecond); err != nil {
				s.log.Warnf("broadcast receipt to conn %s failed: %v", c.ConnID(), err)
				continue
			}
			sent++
		}
	}
	return sent
}

// batchPushConcurrent distributes users across a fixed worker pool.
func (s *PushService) batchPushConcurrent(ctx context.Context, userIDs []int64, marshaled []byte) (int, []int64) {
	workerCount := min(len(userIDs), getPushWorkerCount())

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
			for idx, userID := range ids {
				// Check context cancellation between users
				select {
				case <-ctx.Done():
					// Report all remaining users in this chunk as failed so callers
					// can retry them; otherwise these users would silently disappear.
					for _, uid := range ids[idx:] {
						resultCh <- result{userID: uid, delivered: 0}
					}
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
