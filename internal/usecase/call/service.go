package call

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"

	domaincall "pet-messenger/internal/domain/call"
	"pet-messenger/internal/domain/conversation"
	"pet-messenger/internal/domain/message"
)

type Service struct {
	calls   domaincall.Repository
	convs   conversation.Repository
	nodes   MediaNodeRegistry
	rooms   RoomControl
	tickets JoinTicketIssuer
	ice     ICEConfigProvider
	events  SystemEventPoster
	notif   CallNotifier
}

func NewService(calls domaincall.Repository, convs conversation.Repository, nodes MediaNodeRegistry, rooms RoomControl, tickets JoinTicketIssuer, ice ICEConfigProvider, events SystemEventPoster, notif CallNotifier) *Service {
	return &Service{calls: calls, convs: convs, nodes: nodes, rooms: rooms, tickets: tickets, ice: ice, events: events, notif: notif}
}

func callableKind(k conversation.Kind) bool {
	return k == conversation.KindDirect || k == conversation.KindGroupChat || k == conversation.KindChannelGroup
}

func (s *Service) wsURL(node Node) string { return "ws://" + node.WSAddr + "/rtc" }

func (s *Service) joinInfo(callID, userID uuid.UUID, role domaincall.Role, node Node) (JoinInfo, error) {
	tok, exp, err := s.tickets.Issue(callID, userID, role, node.ID)
	if err != nil {
		return JoinInfo{}, err
	}
	return JoinInfo{
		Ticket:          tok,
		TicketExpiresAt: exp,
		MediaWSURL:      s.wsURL(node),
		ICEServers:      s.ice.ServersFor(userID),
		Role:            role,
	}, nil
}

func (s *Service) StartCall(ctx context.Context, actorID, conversationID uuid.UUID) (*domaincall.Call, JoinInfo, error) {
	conv, err := s.convs.GetByID(ctx, conversationID)
	if err != nil {
		return nil, JoinInfo{}, err // conversation.ErrNotFound
	}
	if !callableKind(conv.Kind) {
		return nil, JoinInfo{}, ErrCallNotSupported
	}
	if conv.IsArchived() {
		return nil, JoinInfo{}, conversation.ErrConversationArchived
	}
	if _, err := s.convs.GetMember(ctx, conversationID, actorID); err != nil {
		return nil, JoinInfo{}, err // conversation.ErrNotMember
	}

	if existing, err := s.calls.GetLiveForConversation(ctx, conversationID); err == nil {
		node, gerr := s.nodes.Get(ctx, existing.MediaNodeID)
		if gerr != nil {
			return nil, JoinInfo{}, ErrMediaNodeGone
		}
		if aerr := s.addParticipant(ctx, existing.ID, actorID, domaincall.RolePublisher); aerr != nil {
			return nil, JoinInfo{}, aerr
		}
		ji, jerr := s.joinInfo(existing.ID, actorID, domaincall.RolePublisher, node)
		return existing, ji, jerr
	} else if !errors.Is(err, domaincall.ErrNotFound) {
		return nil, JoinInfo{}, err
	}

	node, err := s.nodes.PickLeastLoaded(ctx)
	if err != nil {
		return nil, JoinInfo{}, ErrNoMediaNodes
	}
	c, err := domaincall.New(conversationID, actorID, node.ID)
	if err != nil {
		return nil, JoinInfo{}, err
	}
	if err := s.calls.Create(ctx, c); err != nil {
		if errors.Is(err, domaincall.ErrLiveCallExists) {
			won, rerr := s.calls.GetLiveForConversation(ctx, conversationID)
			if rerr != nil {
				return nil, JoinInfo{}, rerr
			}
			node2, gerr := s.nodes.Get(ctx, won.MediaNodeID)
			if gerr != nil {
				return nil, JoinInfo{}, ErrMediaNodeGone
			}
			_ = s.addParticipant(ctx, won.ID, actorID, domaincall.RolePublisher)
			ji, jerr := s.joinInfo(won.ID, actorID, domaincall.RolePublisher, node2)
			return won, ji, jerr
		}
		return nil, JoinInfo{}, err
	}

	if err := s.rooms.CreateRoom(ctx, node, c.ID); err != nil {
		if _, cerr := s.calls.MarkEnded(ctx, c.ID, time.Now().UTC()); cerr != nil {
			log.Printf("call: compensate MarkEnded %s: %v", c.ID, cerr)
		}
		return nil, JoinInfo{}, ErrMediaUnavailable
	}

	if err := s.addParticipant(ctx, c.ID, actorID, domaincall.RolePublisher); err != nil {
		return nil, JoinInfo{}, err
	}
	if err := s.events.PostSystemEvent(ctx, conv, &actorID, message.SystemCallStarted, c.ID, ""); err != nil {
		log.Printf("call: post SystemCallStarted %s: %v", c.ID, err)
	}
	if err := s.notif.CallStarted(ctx, c.ID, conversationID, actorID); err != nil {
		log.Printf("call: notify CallStarted %s: %v", c.ID, err)
	}

	ji, err := s.joinInfo(c.ID, actorID, domaincall.RolePublisher, node)
	return c, ji, err
}

func (s *Service) addParticipant(ctx context.Context, callID, userID uuid.UUID, role domaincall.Role) error {
	p, err := domaincall.NewParticipant(callID, userID, role)
	if err != nil {
		return err
	}
	if err := s.calls.AddParticipant(ctx, p); err != nil && !errors.Is(err, domaincall.ErrAlreadyInCall) {
		return err
	}
	return nil
}

func (s *Service) JoinCall(ctx context.Context, actorID, callID uuid.UUID, requestedRole domaincall.Role) (JoinInfo, error) {
	c, err := s.calls.GetByID(ctx, callID)
	if err != nil {
		return JoinInfo{}, err
	}
	if !c.IsLive() {
		return JoinInfo{}, domaincall.ErrCallNotLive
	}
	if _, err := s.convs.GetMember(ctx, c.ConversationID, actorID); err != nil {
		return JoinInfo{}, err
	}
	role := requestedRole
	if !role.Valid() {
		role = domaincall.RolePublisher
	}
	node, err := s.nodes.Get(ctx, c.MediaNodeID)
	if err != nil {
		if _, merr := s.calls.MarkEnded(ctx, c.ID, time.Now().UTC()); merr != nil {
			log.Printf("call: MarkEnded on gone node %s: %v", c.ID, merr)
		}
		return JoinInfo{}, ErrMediaNodeGone
	}
	if err := s.addParticipant(ctx, c.ID, actorID, role); err != nil {
		return JoinInfo{}, err
	}
	return s.joinInfo(c.ID, actorID, role, node)
}

func (s *Service) GetLiveForConversation(ctx context.Context, actorID, conversationID uuid.UUID) (*domaincall.Call, error) {
	if _, err := s.convs.GetMember(ctx, conversationID, actorID); err != nil {
		return nil, err
	}
	return s.calls.GetLiveForConversation(ctx, conversationID)
}

func (s *Service) authorizeModeration(ctx context.Context, c *domaincall.Call, actorID uuid.UUID) error {
	if actorID == c.InitiatorID {
		return nil
	}
	m, err := s.convs.GetMember(ctx, c.ConversationID, actorID)
	if err != nil {
		return ErrForbidden
	}
	if !m.AdminTier.AtLeast(conversation.TierAdmin) {
		return ErrForbidden
	}
	return nil
}

func (s *Service) EndCall(ctx context.Context, actorID, callID uuid.UUID) error {
	c, err := s.calls.GetByID(ctx, callID)
	if err != nil {
		return err
	}
	if !c.IsLive() {
		return domaincall.ErrAlreadyEnded
	}
	if err := s.authorizeModeration(ctx, c, actorID); err != nil {
		return err
	}
	if node, gerr := s.nodes.Get(ctx, c.MediaNodeID); gerr == nil {
		if cerr := s.rooms.CloseRoom(ctx, node, callID); cerr != nil {
			log.Printf("call: CloseRoom %s: %v", callID, cerr)
		}
	}
	return s.finishCall(ctx, c, &actorID)
}

func (s *Service) finishCall(ctx context.Context, c *domaincall.Call, actorID *uuid.UUID) error {
	transitioned, err := s.calls.MarkEnded(ctx, c.ID, time.Now().UTC())
	if err != nil {
		return err
	}
	if !transitioned {
		return nil
	}
	conv, err := s.convs.GetByID(ctx, c.ConversationID)
	if err != nil {
		log.Printf("call: load conv for SystemCallEnded %s: %v", c.ID, err)
		return nil
	}
	if perr := s.events.PostSystemEvent(ctx, conv, actorID, message.SystemCallEnded, c.ID, ""); perr != nil {
		log.Printf("call: post SystemCallEnded %s: %v", c.ID, perr)
	}
	return nil
}

func (s *Service) MuteParticipant(ctx context.Context, actorID, callID, targetID uuid.UUID) error {
	return s.moderateParticipant(ctx, actorID, callID, targetID, false)
}

func (s *Service) RemoveParticipant(ctx context.Context, actorID, callID, targetID uuid.UUID) error {
	return s.moderateParticipant(ctx, actorID, callID, targetID, true)
}

func (s *Service) moderateParticipant(ctx context.Context, actorID, callID, targetID uuid.UUID, remove bool) error {
	c, err := s.calls.GetByID(ctx, callID)
	if err != nil {
		return err
	}
	if !c.IsLive() {
		return domaincall.ErrCallNotLive
	}
	if targetID == actorID {
		return ErrCannotTargetSelf
	}
	if err := s.authorizeModeration(ctx, c, actorID); err != nil {
		return err
	}
	node, err := s.nodes.Get(ctx, c.MediaNodeID)
	if err != nil {
		return ErrMediaNodeGone
	}
	if remove {
		if err := s.rooms.RemoveParticipant(ctx, node, callID, targetID); err != nil {
			return err
		}
		if err := s.calls.MarkParticipantLeft(ctx, callID, targetID, time.Now().UTC()); err != nil {
			log.Printf("call: MarkParticipantLeft %s/%s: %v", callID, targetID, err)
		}
		return nil
	}
	return s.rooms.MuteParticipant(ctx, node, callID, targetID)
}

func (s *Service) HandleParticipantLeft(ctx context.Context, callID, userID uuid.UUID) error {
	return s.calls.MarkParticipantLeft(ctx, callID, userID, time.Now().UTC())
}

func (s *Service) HandleCallEnded(ctx context.Context, callID uuid.UUID) error {
	c, err := s.calls.GetByID(ctx, callID)
	if err != nil {
		if errors.Is(err, domaincall.ErrNotFound) {
			return nil
		}
		return err
	}
	return s.finishCall(ctx, c, nil)
}
