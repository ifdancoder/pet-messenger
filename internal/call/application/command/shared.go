// Package command holds the CQRS write side of the call context: each
// exported New*Handler wires one command. Several commands share private
// helpers declared in this file rather than duplicating logic across
// handler types in the same package.
package command

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"

	out "pet-messenger/internal/call/application/port/out"
	domain "pet-messenger/internal/call/domain"
	conversation "pet-messenger/internal/conversation/domain"
	domainmsg "pet-messenger/internal/message/domain"
)

// Sentinel errors returned by this package's commands.
var (
	// ErrCallNotSupported means the conversation's kind cannot host a
	// call (only direct, group chat, and channel-group conversations
	// can).
	ErrCallNotSupported = errors.New("call: this conversation kind cannot host a call")
	// ErrModerationNotSupported means moderation (mute/unmute/remove) is
	// unavailable in a direct call.
	ErrModerationNotSupported = errors.New("call: moderation is unavailable in a direct call")
	// ErrForbidden covers every authorization failure this package's
	// commands can hit: a non-initiator, non-admin actor ending or
	// moderating a call.
	ErrForbidden = errors.New("call: not allowed")
	// ErrCannotTargetSelf means a moderation action targeted the actor
	// themself.
	ErrCannotTargetSelf = errors.New("call: cannot target yourself")
	// ErrMediaUnavailable means the SFU rejected a room-create request.
	ErrMediaUnavailable = errors.New("call: no media capacity right now")
	// ErrMediaNodeGone means the media node a live call was assigned to
	// is no longer in the registry.
	ErrMediaNodeGone = errors.New("call: the media node for this call is gone")
	// ErrNoMediaNodes means the media-node registry has nothing to
	// assign a new call to.
	ErrNoMediaNodes = errors.New("call: no media nodes registered")
)

// callableKind reports whether a conversation of kind k may host a call.
func callableKind(k conversation.Kind) bool {
	return k == conversation.KindDirect || k == conversation.KindGroupChat || k == conversation.KindChannelGroup
}

// wsURL builds a call's media-node WebSocket URL.
func wsURL(node out.Node) string { return "ws://" + node.WSAddr + "/rtc" }

// joinInfo issues a join ticket and ICE config for userID to join callID
// on node.
func joinInfo(tickets out.JoinTicketIssuer, ice out.ICEConfigProvider, callID, userID uuid.UUID, role domain.Role, node out.Node) (out.JoinInfo, error) {
	tok, exp, err := tickets.Issue(callID, userID, role, node.ID)
	if err != nil {
		return out.JoinInfo{}, err
	}
	return out.JoinInfo{
		Ticket:          tok,
		TicketExpiresAt: exp,
		MediaWSURL:      wsURL(node),
		ICEServers:      ice.ServersFor(userID),
		Role:            role,
	}, nil
}

// addParticipant inserts a participant row for userID, tolerating a race
// where it is already there (domain.ErrAlreadyInCall).
func addParticipant(ctx context.Context, writer out.ParticipantWriter, callID, userID uuid.UUID, role domain.Role) error {
	p, err := domain.NewParticipant(callID, userID, role)
	if err != nil {
		return err
	}
	if err := writer.AddParticipant(ctx, p); err != nil && !errors.Is(err, domain.ErrAlreadyInCall) {
		return err
	}
	return nil
}

// authorizeModeration allows a call's initiator, or a conversation member
// at admin tier or above, and forbids everyone else.
func authorizeModeration(ctx context.Context, convs out.ConversationReader, c *domain.Call, actorID uuid.UUID) error {
	if actorID == c.InitiatorID {
		return nil
	}
	m, err := convs.GetMember(ctx, c.ConversationID, actorID)
	if err != nil {
		return ErrForbidden
	}
	if !m.AdminTier.AtLeast(conversation.TierAdmin) {
		return ErrForbidden
	}
	return nil
}

// finishCall marks c ended and posts the call_ended timeline message,
// exactly once even if called twice for the same call (MarkEnded's
// transitioned bool guards the double-post). Shared by EndCall,
// HandleCallEnded, and ReapStaleCalls in this package.
//
// A SEPARATE, independent copy of this same logic lives in
// internal/call/application/query/get_live_for_conversation.go, for that
// query's own lazy-cleanup use. Go gives no way to share unexported code
// across sibling packages without a third shared package, which this
// project has consistently declined to add (see e2ee's own
// callForMember/callKeyScope precedent). If you change this function,
// change the query package's copy too.
func finishCall(ctx context.Context, calls out.CallWriter, convs out.ConversationReader, events out.SystemEventPoster, c *domain.Call, actorID *uuid.UUID) error {
	transitioned, err := calls.MarkEnded(ctx, c.ID, time.Now().UTC())
	if err != nil {
		return err
	}
	if !transitioned {
		return nil
	}
	conv, err := convs.GetByID(ctx, c.ConversationID)
	if err != nil {
		log.Printf("call: load conv for SystemCallEnded %s: %v", c.ID, err)
		return nil
	}
	if perr := events.PostSystemEvent(ctx, conv, actorID, domainmsg.SystemCallEnded, c.ID, ""); perr != nil {
		log.Printf("call: post SystemCallEnded %s: %v", c.ID, perr)
	}
	return nil
}

// moderateParticipant runs the shared preconditions for mute/unmute/remove
// (call must be live, conversation must support moderation, actor cannot
// target itself, actor must be authorized), then calls action with the
// call's media node.
func moderateParticipant(ctx context.Context, calls out.CallLoader, convs out.ConversationReader, nodes out.MediaNodeRegistry, actorID, callID, targetID uuid.UUID, action func(context.Context, out.Node, uuid.UUID, uuid.UUID) error) error {
	c, err := calls.GetByID(ctx, callID)
	if err != nil {
		return err
	}
	if !c.IsLive() {
		return domain.ErrCallNotLive
	}
	conv, err := convs.GetByID(ctx, c.ConversationID)
	if err != nil {
		return err
	}
	if conv.Kind == conversation.KindDirect {
		return ErrModerationNotSupported
	}
	if targetID == actorID {
		return ErrCannotTargetSelf
	}
	if err := authorizeModeration(ctx, convs, c, actorID); err != nil {
		return err
	}
	node, err := nodes.Get(ctx, c.MediaNodeID)
	if err != nil {
		return ErrMediaNodeGone
	}
	return action(ctx, node, callID, targetID)
}
