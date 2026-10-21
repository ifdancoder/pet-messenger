package rpc

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"pet-messenger/internal/platform/adapter/rpc/mediav1"
	"pet-messenger/internal/platform/adapter/sfu"
)

type RoomControlServer struct {
	mediav1.UnimplementedRoomControlServer
	mgr sfu.Manager
}

func NewRoomControlServer(m sfu.Manager) *RoomControlServer { return &RoomControlServer{mgr: m} }

func parseID(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, "bad id: "+s)
	}
	return id, nil
}

func (s *RoomControlServer) CreateRoom(_ context.Context, r *mediav1.CreateRoomRequest) (*mediav1.CreateRoomResponse, error) {
	id, err := parseID(r.CallId)
	if err != nil {
		return nil, err
	}
	if err := s.mgr.CreateRoom(id); err != nil {
		if err == sfu.ErrNodeAtCapacity {
			return nil, status.Error(codes.ResourceExhausted, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &mediav1.CreateRoomResponse{}, nil
}

func (s *RoomControlServer) CloseRoom(_ context.Context, r *mediav1.CloseRoomRequest) (*mediav1.CloseRoomResponse, error) {
	id, err := parseID(r.CallId)
	if err != nil {
		return nil, err
	}
	_ = s.mgr.CloseRoom(id)
	return &mediav1.CloseRoomResponse{}, nil
}

func (s *RoomControlServer) MuteParticipant(_ context.Context, r *mediav1.MuteParticipantRequest) (*mediav1.MuteParticipantResponse, error) {
	cid, err := parseID(r.CallId)
	if err != nil {
		return nil, err
	}
	uid, err := parseID(r.UserId)
	if err != nil {
		return nil, err
	}
	if err := s.mgr.Mute(cid, uid); err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}
	return &mediav1.MuteParticipantResponse{}, nil
}

func (s *RoomControlServer) UnmuteParticipant(_ context.Context, r *mediav1.UnmuteParticipantRequest) (*mediav1.UnmuteParticipantResponse, error) {
	cid, err := parseID(r.CallId)
	if err != nil {
		return nil, err
	}
	uid, err := parseID(r.UserId)
	if err != nil {
		return nil, err
	}
	if err := s.mgr.Unmute(cid, uid); err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}
	return &mediav1.UnmuteParticipantResponse{}, nil
}

func (s *RoomControlServer) RemoveParticipant(_ context.Context, r *mediav1.RemoveParticipantRequest) (*mediav1.RemoveParticipantResponse, error) {
	cid, err := parseID(r.CallId)
	if err != nil {
		return nil, err
	}
	uid, err := parseID(r.UserId)
	if err != nil {
		return nil, err
	}
	if err := s.mgr.Remove(cid, uid); err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}
	return &mediav1.RemoveParticipantResponse{}, nil
}

func (s *RoomControlServer) RoomStats(_ context.Context, r *mediav1.RoomStatsRequest) (*mediav1.RoomStatsResponse, error) {
	id, err := parseID(r.CallId)
	if err != nil {
		return nil, err
	}
	st := s.mgr.Stats(id)
	return &mediav1.RoomStatsResponse{Participants: uint32(st.Participants), Exists: st.Exists}, nil
}

func (s *RoomControlServer) ListParticipants(_ context.Context, r *mediav1.ListParticipantsRequest) (*mediav1.ListParticipantsResponse, error) {
	id, err := parseID(r.CallId)
	if err != nil {
		return nil, err
	}
	peers, ok := s.mgr.ListParticipants(id)
	if !ok {
		return nil, status.Error(codes.NotFound, "room not found")
	}
	out := make([]*mediav1.Participant, 0, len(peers))
	for _, p := range peers {
		seen := make(map[string]bool, len(p.Tracks))
		kindList := make([]string, 0, len(p.Tracks))
		for _, tr := range p.Tracks {
			if seen[tr.Label] {
				continue
			}
			seen[tr.Label] = true
			kindList = append(kindList, tr.Label)
		}
		out = append(out, &mediav1.Participant{
			UserId: p.UserID, Role: p.Role, Muted: p.Muted, SelfMuted: p.SelfMuted, TrackKinds: kindList,
		})
	}
	return &mediav1.ListParticipantsResponse{Participants: out}, nil
}
