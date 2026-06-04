package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/proto"
	pb "nonoka-im/api/im/v1"
	"nonoka-im/internal/gateway"
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

	payload, err := proto.Marshal(msg)
	if err != nil {
		s.log.Errorf("marshal push message failed: %v", err)
		return &pb.PushToUserReply{Success: false, DeliveredCount: 0}, nil
	}

	packet := &pb.Packet{
		Cmd:     pb.Command_CMD_NOTIFY,
		Payload: payload,
	}

	delivered := s.manager.BroadcastToUser(req.GetUserId(), packet)
	s.log.Debugf("push to user: user_id=%d, delivered=%d", req.GetUserId(), delivered)

	return &pb.PushToUserReply{
		Success:        delivered > 0,
		DeliveredCount: int32(delivered),
	}, nil
}

// BatchPushToUsers delivers messages to multiple users in one call.
func (s *PushService) BatchPushToUsers(ctx context.Context, req *pb.BatchPushToUsersRequest) (*pb.BatchPushToUsersReply, error) {
	msg := req.GetMessage()
	if msg == nil {
		return &pb.BatchPushToUsersReply{TotalDelivered: 0}, nil
	}

	payload, err := proto.Marshal(msg)
	if err != nil {
		s.log.Errorf("marshal batch push message failed: %v", err)
		return &pb.BatchPushToUsersReply{TotalDelivered: 0}, nil
	}

	packet := &pb.Packet{
		Cmd:     pb.Command_CMD_NOTIFY,
		Payload: payload,
	}

	totalDelivered := 0
	var failedUserIDs []int64

	for _, userID := range req.GetUserIds() {
		delivered := s.manager.BroadcastToUser(userID, packet)
		if delivered == 0 {
			failedUserIDs = append(failedUserIDs, userID)
		}
		totalDelivered += delivered
	}

	s.log.Debugf("batch push: users=%d, total_delivered=%d", len(req.GetUserIds()), totalDelivered)

	return &pb.BatchPushToUsersReply{
		TotalDelivered:  int32(totalDelivered),
		FailedUserIds:   failedUserIDs,
	}, nil
}
