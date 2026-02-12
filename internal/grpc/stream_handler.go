package grpc

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/ambientlabscomputing/mycelium_spine/internal/service"
	"github.com/ambientlabscomputing/mycelium_spine/internal/types"
	"github.com/ambientlabscomputing/mycelium_spine/internal/utils"
	umsv1 "github.com/ambientlabscomputing/mycelium_spine/proto/ums/v1"
)

// StreamHandler implements SpineStreamServer
type StreamHandler struct {
	umsv1.UnimplementedSpineStreamServer
	appService service.Service
	logger     *slog.Logger
}

// NewStreamHandler creates a new stream handler
func NewStreamHandler(appService service.Service) *StreamHandler {
	return &StreamHandler{
		appService: appService,
		logger:     utils.Logger.With("handler", "stream"),
	}
}

// Connect implements the bidirectional streaming RPC
func (h *StreamHandler) Connect(stream umsv1.SpineStream_ConnectServer) error {
	ctx := stream.Context()
	logger := h.logger.With("remote_addr", "todo") // TODO: extract from context

	logger.Info("new connection established")

	// Wait for HELLO frame (must be first)
	firstFrame, err := stream.Recv()
	if err != nil {
		logger.Error("failed to receive first frame", "error", err)
		return err
	}

	helloFrame := firstFrame.GetHello()
	if helloFrame == nil {
		logger.Error("first frame was not HELLO")
		h.sendError(stream, "PROTOCOL_ERROR", "first frame must be HELLO", false)
		return fmt.Errorf("first frame must be HELLO")
	}

	// Process HELLO and create/resume session
	var session *types.Session
	sessionSvc := h.appService.GetSessionService()

	if helloFrame.ResumeToken != "" {
		// Session resumption
		resumeOk, resumedSession, err := sessionSvc.HandleResume(ctx, helloFrame.ResumeToken)
		if err != nil {
			logger.Warn("session resume failed", "error", err)
			// Send RESUME_DENIED
			h.sendResumeDenied(stream, err.Error())
			// Fall through to new session creation
		} else {
			// Send RESUME_OK
			if err := h.sendResumeOk(stream, resumeOk); err != nil {
				return err
			}
			session = resumedSession
			logger.Info("session resumed", "session_id", session.SessionID)
		}
	}

	// If no session yet (no resume token or resume failed), create new session
	if session == nil {
		welcomeFrame, newSession, err := sessionSvc.HandleHello(ctx, helloFrame)
		if err != nil {
			logger.Error("failed to handle HELLO", "error", err)
			h.sendError(stream, "SESSION_ERROR", err.Error(), false)
			return err
		}

		// Send WELCOME
		if err := h.sendWelcome(stream, welcomeFrame); err != nil {
			return err
		}

		session = newSession
		logger.Info("new session created", "session_id", session.SessionID)
	}

	// Attach stream to session
	session.Stream = stream

	// Register session
	if err := sessionSvc.RegisterSession(ctx, session); err != nil {
		logger.Error("failed to register session", "error", err)
		return err
	}

	// Start delivery loop
	deliverySvc := h.appService.GetDeliveryService()
	if err := deliverySvc.StartDeliveryLoop(ctx, session); err != nil {
		logger.Error("failed to start delivery loop", "error", err)
		return err
	}

	// Main message loop
	defer func() {
		logger.Info("connection closing", "session_id", session.SessionID)
		deliverySvc.StopDeliveryLoop(ctx, session.SessionID)
		sessionSvc.UnregisterSession(ctx, session.SessionID)
	}()

	for {
		frame, err := stream.Recv()
		if err == io.EOF {
			logger.Info("client closed connection", "session_id", session.SessionID)
			return nil
		}
		if err != nil {
			logger.Error("error receiving frame", "error", err, "session_id", session.SessionID)
			return err
		}

		// Dispatch frame to appropriate handler
		if err := h.handleClientFrame(ctx, session, frame); err != nil {
			logger.Error("error handling frame", "error", err, "session_id", session.SessionID)
			h.sendError(stream, "PROCESSING_ERROR", err.Error(), true)
			// Continue processing (retryable error)
		}
	}
}

// handleClientFrame dispatches a client frame to the appropriate service
func (h *StreamHandler) handleClientFrame(ctx context.Context, session *types.Session, frame *umsv1.ClientFrame) error {
	switch f := frame.Frame.(type) {
	case *umsv1.ClientFrame_Ack:
		return h.appService.GetAckService().HandleCumulativeAck(
			ctx,
			session.SessionID,
			f.Ack.MailboxId,
			f.Ack.SeqAcked,
		)

	case *umsv1.ClientFrame_AckSet:
		return h.appService.GetAckService().HandleSelectiveAck(
			ctx,
			session.SessionID,
			f.AckSet.MailboxId,
			f.AckSet.Seqs,
		)

	case *umsv1.ClientFrame_Subscribe:
		targets := make([]*types.Target, len(f.Subscribe.Targets))
		for i, protoTarget := range f.Subscribe.Targets {
			targets[i] = &types.Target{
				TargetType:  protoTargetTypeToTypes(protoTarget.TargetType),
				TargetID:    protoTarget.TargetId,
				OrgID:       protoTarget.OrgId,
				DeliverRole: protoTarget.DeliverRole,
			}
		}

		subscribeOk, err := h.appService.GetSubscribeService().HandleSubscribe(ctx, session, targets)
		if err != nil {
			return err
		}

		return h.sendSubscribeOk(session.Stream, subscribeOk)

	case *umsv1.ClientFrame_Ping:
		// Update heartbeat
		if err := h.appService.GetSessionService().HandleHeartbeat(ctx, session.SessionID); err != nil {
			return err
		}
		// Send PONG
		return h.sendPong(session.Stream, f.Ping.ClientTimeMs)

	case *umsv1.ClientFrame_FlowHint:
		return h.appService.GetDeliveryService().HandleBackpressure(ctx, session, f.FlowHint)

	case *umsv1.ClientFrame_Nack:
		return h.appService.GetAckService().HandleNack(
			ctx,
			session.SessionID,
			f.Nack.MailboxId,
			f.Nack.Seq,
			f.Nack.Reason,
		)

	default:
		return fmt.Errorf("unknown frame type")
	}
}

// Helper methods for sending server frames

func (h *StreamHandler) sendWelcome(stream umsv1.SpineStream_ConnectServer, welcome *umsv1.WelcomeFrame) error {
	return stream.Send(&umsv1.ServerFrame{
		Frame: &umsv1.ServerFrame_Welcome{Welcome: welcome},
	})
}

func (h *StreamHandler) sendResumeOk(stream umsv1.SpineStream_ConnectServer, resumeOk *umsv1.ResumeOkFrame) error {
	return stream.Send(&umsv1.ServerFrame{
		Frame: &umsv1.ServerFrame_ResumeOk{ResumeOk: resumeOk},
	})
}

func (h *StreamHandler) sendResumeDenied(stream umsv1.SpineStream_ConnectServer, reason string) error {
	return stream.Send(&umsv1.ServerFrame{
		Frame: &umsv1.ServerFrame_ResumeDenied{
			ResumeDenied: &umsv1.ResumeDeniedFrame{Reason: reason},
		},
	})
}

func (h *StreamHandler) sendSubscribeOk(stream umsv1.SpineStream_ConnectServer, subscribeOk *umsv1.SubscribeOkFrame) error {
	return stream.Send(&umsv1.ServerFrame{
		Frame: &umsv1.ServerFrame_SubscribeOk{SubscribeOk: subscribeOk},
	})
}

func (h *StreamHandler) sendPong(stream umsv1.SpineStream_ConnectServer, clientTimeMs int64) error {
	return stream.Send(&umsv1.ServerFrame{
		Frame: &umsv1.ServerFrame_Pong{
			Pong: &umsv1.PongFrame{
				ServerTimeMs: 0, // TODO: current time in ms
				ClientTimeMs: clientTimeMs,
			},
		},
	})
}

func (h *StreamHandler) sendError(stream umsv1.SpineStream_ConnectServer, code string, message string, retryable bool) error {
	return stream.Send(&umsv1.ServerFrame{
		Frame: &umsv1.ServerFrame_Error{
			Error: &umsv1.ErrorFrame{
				Code:      code,
				Message:   message,
				Retryable: retryable,
			},
		},
	})
}

// protoTargetTypeToTypes converts proto TargetType to internal type
func protoTargetTypeToTypes(proto umsv1.TargetType) types.TargetType {
	switch proto {
	case umsv1.TargetType_TARGET_TYPE_SERVER:
		return types.TargetTypeServer
	case umsv1.TargetType_TARGET_TYPE_CLUSTER:
		return types.TargetTypeCluster
	case umsv1.TargetType_TARGET_TYPE_ORG:
		return types.TargetTypeOrg
	case umsv1.TargetType_TARGET_TYPE_SERVICE:
		return types.TargetTypeService
	case umsv1.TargetType_TARGET_TYPE_BROADCAST:
		return types.TargetTypeBroadcast
	default:
		return types.TargetTypeUnspecified
	}
}
