package call

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"

	domaincall "pet-messenger/internal/domain/call"
	"pet-messenger/internal/domain/message"

	conversation "pet-messenger/internal/conversation/domain"
)

type Service struct {
	calls   domaincall.Repository
	convs   ConversationReader
	nodes   MediaNodeRegistry
	rooms   RoomControl
	tickets JoinTicketIssuer
	ice     ICEConfigProvider
	events  SystemEventPoster
	notif   CallNotifier
}

func NewService(calls domaincall.Repository, convs ConversationReader, nodes MediaNodeRegistry, rooms RoomControl, tickets JoinTicketIssuer, ice ICEConfigProvider, events SystemEventPoster, notif CallNotifier) *Service {
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

func (s *Service) GetLiveForConversation(ctx context.Context, actorID, conversationID uuid.UUID) (*domaincall.Call, []ParticipantInfo, error) {
	if _, err := s.convs.GetMember(ctx, conversationID, actorID); err != nil {
		return nil, nil, err
	}
	c, err := s.calls.GetLiveForConversation(ctx, conversationID)
	if err != nil {
		return nil, nil, err
	}
	node, err := s.nodes.Get(ctx, c.MediaNodeID)
	if err != nil {
		return c, []ParticipantInfo{}, nil
	}
	participants, err := s.rooms.ListParticipants(ctx, node, c.ID)
	if err != nil {
		if errors.Is(err, ErrRoomNotFound) {
			if ferr := s.finishCall(ctx, c, nil); ferr != nil {
				return nil, nil, ferr
			}
			return nil, nil, domaincall.ErrNotFound
		}
		return c, []ParticipantInfo{}, nil
	}
	return c, s.withParticipationHistory(ctx, c.ID, participants), nil
}

// withParticipationHistory keeps the live roster useful for call UI after a
// participant leaves. A publisher who already accepted must not become a
// fresh ringing invitation merely because their current SFU session ended.
// Viewer rows are intentionally excluded: opening a docked preview is not
// accepting the call.
func (s *Service) withParticipationHistory(ctx context.Context, callID uuid.UUID, live []ParticipantInfo) []ParticipantInfo {
	history, err := s.calls.ListParticipants(ctx, callID)
	if err != nil {
		log.Printf("call: list participation history %s: %v", callID, err)
		return live
	}
	accepted := make(map[string]bool, len(history))
	for _, participant := range history {
		if participant.Role == domaincall.RolePublisher {
			accepted[participant.UserID.String()] = true
		}
	}
	connected := make(map[string]bool, len(live))
	for i := range live {
		live[i].Connected = true
		live[i].Accepted = accepted[live[i].UserID]
		connected[live[i].UserID] = true
	}
	for userID := range accepted {
		if !connected[userID] {
			live = append(live, ParticipantInfo{UserID: userID, Role: string(domaincall.RolePublisher), TrackKinds: []string{}, Accepted: true})
		}
	}
	return live
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
	conv, err := s.convs.GetByID(ctx, c.ConversationID)
	if err != nil {
		return err
	}
	if conv.Kind == conversation.KindDirect {
		if _, err := s.convs.GetMember(ctx, c.ConversationID, actorID); err != nil {
			return ErrForbidden
		}
	} else if err := s.authorizeModeration(ctx, c, actorID); err != nil {
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

// ReapStaleCalls is the hard-crash backstop: a media pod that dies without
// ever publishing call.<id>.ended leaves its calls stuck 'active' forever.
// Called periodically, it treats a live call whose node is no longer in the
// registry as dead and closes it server-side.
func (s *Service) ReapStaleCalls(ctx context.Context) (int, error) {
	live, err := s.calls.ListLive(ctx)
	if err != nil {
		return 0, err
	}
	ended := 0
	for _, c := range live {
		node, gerr := s.nodes.Get(ctx, c.MediaNodeID)
		if gerr != nil {
			if err := s.finishCall(ctx, c, nil); err != nil {
				log.Printf("call: reap %s: %v", c.ID, err)
				continue
			}
			ended++
			continue
		}
		if _, rerr := s.rooms.ListParticipants(ctx, node, c.ID); errors.Is(rerr, ErrRoomNotFound) {
			if err := s.finishCall(ctx, c, nil); err != nil {
				log.Printf("call: reap missing room %s: %v", c.ID, err)
				continue
			}
			ended++
		}
	}
	return ended, nil
}

func (s *Service) MuteParticipant(ctx context.Context, actorID, callID, targetID uuid.UUID) error {
	return s.moderateParticipant(ctx, actorID, callID, targetID, s.rooms.MuteParticipant)
}

func (s *Service) UnmuteParticipant(ctx context.Context, actorID, callID, targetID uuid.UUID) error {
	return s.moderateParticipant(ctx, actorID, callID, targetID, s.rooms.UnmuteParticipant)
}

func (s *Service) RemoveParticipant(ctx context.Context, actorID, callID, targetID uuid.UUID) error {
	return s.moderateParticipant(ctx, actorID, callID, targetID, func(ctx context.Context, node Node, callID, targetID uuid.UUID) error {
		if err := s.rooms.RemoveParticipant(ctx, node, callID, targetID); err != nil {
			return err
		}
		if err := s.calls.MarkParticipantLeft(ctx, callID, targetID, time.Now().UTC()); err != nil {
			log.Printf("call: MarkParticipantLeft %s/%s: %v", callID, targetID, err)
		}
		return nil
	})
}

func (s *Service) moderateParticipant(ctx context.Context, actorID, callID, targetID uuid.UUID, action func(context.Context, Node, uuid.UUID, uuid.UUID) error) error {
	c, err := s.calls.GetByID(ctx, callID)
	if err != nil {
		return err
	}
	if !c.IsLive() {
		return domaincall.ErrCallNotLive
	}
	conv, err := s.convs.GetByID(ctx, c.ConversationID)
	if err != nil {
		return err
	}
	if conv.Kind == conversation.KindDirect {
		return ErrModerationNotSupported
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
	return action(ctx, node, callID, targetID)
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
