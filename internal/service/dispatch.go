package service

import (
	"context"

	pb "nonoka-im/api/im/v1"
)

type DispatchService struct {
	pb.UnimplementedDispatchServiceServer
}

func NewDispatchService() *DispatchService {
	return &DispatchService{}
}

func (s *DispatchService) Gateway(ctx context.Context, req *pb.GetGatewayRequest) (*pb.GetGatewayReply, error) {
	// TODO: implement gateway dispatch logic (consistent hashing, service discovery, etc.)
	// For now, return a placeholder pointing to the local WebSocket endpoint.
	return &pb.GetGatewayReply{
		GatewayUrl: "ws://localhost:8000/ws",
	}, nil
}