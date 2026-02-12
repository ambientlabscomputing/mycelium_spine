package grpc

import (
	"context"
	"log/slog"

	"github.com/ambientlabscomputing/mycelium_spine/internal/service"
	"github.com/ambientlabscomputing/mycelium_spine/internal/types"
	"github.com/ambientlabscomputing/mycelium_spine/internal/utils"
	umsv1 "github.com/ambientlabscomputing/mycelium_spine/proto/ums/v1"
)

// PublishHandler implements SpinePublishServer
type PublishHandler struct {
	umsv1.UnimplementedSpinePublishServer
	appService service.Service
	logger     *slog.Logger
}

// NewPublishHandler creates a new publish handler
func NewPublishHandler(appService service.Service) *PublishHandler {
	return &PublishHandler{
		appService: appService,
		logger:     utils.Logger.With("handler", "publish"),
	}
}

// Publish handles envelope publishing from external services (server_api, UCRS)
func (h *PublishHandler) Publish(ctx context.Context, req *umsv1.PublishRequest) (*umsv1.PublishResponse, error) {
	logger := h.logger.With("envelope_id", req.Envelope.EnvelopeId)
	logger.Info("received PublishRequest", "target_count", len(req.Targets))

	// TODO: Validate authentication (mTLS or JWT from server_api/UCRS)

	// Convert proto envelope to internal type
	envelope := protoToEnvelope(req.Envelope)

	// Convert proto targets to internal type
	targets := make([]*types.Target, len(req.Targets))
	for i, protoTarget := range req.Targets {
		targets[i] = &types.Target{
			TargetType:  protoTargetTypeToTypes(protoTarget.TargetType),
			TargetID:    protoTarget.TargetId,
			OrgID:       protoTarget.OrgId,
			DeliverRole: protoTarget.DeliverRole,
		}
	}

	// Publish via service layer
	publishSvc := h.appService.GetPublishService()
	result, err := publishSvc.Publish(ctx, envelope, targets)
	if err != nil {
		logger.Error("publish failed", "error", err)
		return &umsv1.PublishResponse{
			Success: false,
			Error:   err.Error(),
		}, nil // Don't return gRPC error, embed error in response
	}

	logger.Info("publish successful", "mailboxes_written", len(result.MailboxSeqs))
	return &umsv1.PublishResponse{
		Success:     true,
		MailboxSeqs: result.MailboxSeqs,
	}, nil
}

// protoToEnvelope converts a proto Envelope to internal type
func protoToEnvelope(proto *umsv1.Envelope) *types.Envelope {
	return &types.Envelope{
		EnvelopeID:  proto.EnvelopeId,
		MailboxID:   proto.MailboxId,
		Seq:         proto.Seq,
		QoS:         protoToQoS(proto.Qos),
		Type:        proto.Type,
		CreatedAtMs: proto.CreatedAtMs,
		ExpiresAtMs: proto.ExpiresAtMs,
		TraceID:     proto.TraceId,
		Payload:     proto.Payload,
		DedupeKey:   proto.DedupeKey,
		Priority:    proto.Priority,
		RequiresAck: proto.RequiresAck,
		OrgID:       proto.OrgId,
	}
}

// protoToQoS converts proto QoS to internal type
func protoToQoS(proto umsv1.QoS) types.QoS {
	switch proto {
	case umsv1.QoS_QOS_COMMAND:
		return types.QoSCommand
	case umsv1.QoS_QOS_CONTROL:
		return types.QoSControl
	case umsv1.QoS_QOS_TELEMETRY:
		return types.QoSTelemetry
	default:
		return types.QoSUnspecified
	}
}
