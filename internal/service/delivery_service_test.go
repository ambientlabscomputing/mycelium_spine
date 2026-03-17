package service

import (
	"context"
	"sync"
	"testing"

	"github.com/ambientlabscomputing/mycelium_spine/internal/types"
	"github.com/ambientlabscomputing/mycelium_spine/internal/utils"
	umsv1 "github.com/ambientlabscomputing/mycelium_spine/proto/ums/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

// mockStream implements umsv1.SpineStream_ConnectServer (grpc.BidiStreamingServer)
type mockStream struct {
	mu       sync.Mutex
	sent     []any
	sendErr  error
	recvFunc func() (*umsv1.ClientFrame, error)
}

func (m *mockStream) Send(msg *umsv1.ServerFrame) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return m.sendErr
}

func (m *mockStream) Recv() (*umsv1.ClientFrame, error) {
	if m.recvFunc != nil {
		return m.recvFunc()
	}
	return nil, nil
}

func (m *mockStream) SetHeader(metadata.MD) error  { return nil }
func (m *mockStream) SendHeader(metadata.MD) error { return nil }
func (m *mockStream) SetTrailer(metadata.MD)       {}
func (m *mockStream) Context() context.Context     { return context.Background() }
func (m *mockStream) SendMsg(msg any) error        { return m.Send(msg.(*umsv1.ServerFrame)) }
func (m *mockStream) RecvMsg(any) error            { return nil }

func newTestSettings() *utils.Settings {
	s := &utils.Settings{}
	s.FlowControl.MaxInflightTotal = 1000
	s.FlowControl.MaxInflightCommand = 100
	s.FlowControl.MaxInflightControl = 100
	s.FlowControl.MaxInflightTelemetry = 100
	return s
}

func TestDeliverFromMailbox_SkipsAlreadyDelivered(t *testing.T) {
	// Scenario: deliverFromMailbox is called twice in rapid succession before any ACK.
	// The second call should NOT re-deliver the same envelope.
	mailboxID := "mbox-1"
	serverID := "server-1"

	var fetchCalls []uint64 // track fromSeq of each FetchEnvelopes call
	var mu sync.Mutex

	repo := &MockRepository{
		GetAckPositionsFunc: func(ctx context.Context, sid string) (map[string]uint64, error) {
			return map[string]uint64{mailboxID: 10}, nil // acked up to 10
		},
		FetchEnvelopesFunc: func(ctx context.Context, mID string, fromSeq uint64, limit int) ([]*types.Envelope, error) {
			mu.Lock()
			fetchCalls = append(fetchCalls, fromSeq)
			mu.Unlock()
			if fromSeq == 11 {
				return []*types.Envelope{
					{EnvelopeID: "env-11", MailboxID: mailboxID, Seq: 11, QoS: types.QoSCommand, Type: "cmd"},
				}, nil
			}
			return nil, nil // nothing above seq 11
		},
	}

	stream := &mockStream{}
	session := types.NewSession(serverID, "org-1", 1, "fp", nil, stream)
	session.Subscriptions = []string{mailboxID}

	svc := NewDeliveryService(repo, nil, newTestSettings(), testMetrics).(*deliveryServiceImpl)
	deliveredUpTo := make(map[string]uint64)

	// First call: should deliver env-11
	svc.deliverFromMailbox(context.Background(), session, mailboxID, deliveredUpTo)

	require.Len(t, fetchCalls, 1, "should have fetched once")
	assert.Equal(t, uint64(11), fetchCalls[0], "first fetch should start from ackPos+1 = 11")
	assert.Equal(t, uint64(11), deliveredUpTo[mailboxID], "deliveredUpTo should advance to 11")
	assert.Len(t, stream.sent, 1, "should have sent one DELIVER frame")

	// Second call: ack position unchanged (no ACK yet), but deliveredUpTo = 11
	svc.deliverFromMailbox(context.Background(), session, mailboxID, deliveredUpTo)

	require.Len(t, fetchCalls, 2, "should have fetched a second time")
	assert.Equal(t, uint64(12), fetchCalls[1], "second fetch should start from deliveredUpTo+1 = 12")
	assert.Len(t, stream.sent, 1, "no new DELIVER frame (nothing at seq 12)")
}

func TestDeliverFromMailbox_AckAdvancesTakesPrecedence(t *testing.T) {
	// When ACK advances past deliveredUpTo, the ack position should dominate.
	mailboxID := "mbox-1"
	serverID := "server-1"
	ackPos := uint64(10)

	var fetchCalls []uint64
	var mu sync.Mutex

	repo := &MockRepository{
		GetAckPositionsFunc: func(ctx context.Context, sid string) (map[string]uint64, error) {
			return map[string]uint64{mailboxID: ackPos}, nil
		},
		FetchEnvelopesFunc: func(ctx context.Context, mID string, fromSeq uint64, limit int) ([]*types.Envelope, error) {
			mu.Lock()
			fetchCalls = append(fetchCalls, fromSeq)
			mu.Unlock()
			if fromSeq == 11 {
				return []*types.Envelope{
					{EnvelopeID: "env-11", MailboxID: mailboxID, Seq: 11, QoS: types.QoSCommand, Type: "cmd"},
				}, nil
			}
			if fromSeq == 12 {
				return []*types.Envelope{
					{EnvelopeID: "env-12", MailboxID: mailboxID, Seq: 12, QoS: types.QoSCommand, Type: "cmd"},
				}, nil
			}
			return nil, nil
		},
	}

	stream := &mockStream{}
	session := types.NewSession(serverID, "org-1", 1, "fp", nil, stream)
	session.Subscriptions = []string{mailboxID}

	svc := NewDeliveryService(repo, nil, newTestSettings(), testMetrics).(*deliveryServiceImpl)
	deliveredUpTo := make(map[string]uint64)

	// First call delivers env-11
	svc.deliverFromMailbox(context.Background(), session, mailboxID, deliveredUpTo)
	assert.Equal(t, uint64(11), deliveredUpTo[mailboxID])

	// Simulate ACK advancing past deliveredUpTo: ackPos jumps to 11
	ackPos = 11

	// Second call: ackPos=11, deliveredUpTo=11 → startSeq=11, fetch from 12
	svc.deliverFromMailbox(context.Background(), session, mailboxID, deliveredUpTo)
	require.Len(t, fetchCalls, 2)
	assert.Equal(t, uint64(12), fetchCalls[1], "should fetch from max(ack=11, delivered=11)+1 = 12")
	assert.Equal(t, uint64(12), deliveredUpTo[mailboxID])
}

func TestDeliverFromMailbox_FlowControlRespected(t *testing.T) {
	mailboxID := "mbox-1"
	serverID := "server-1"

	repo := &MockRepository{
		GetAckPositionsFunc: func(ctx context.Context, sid string) (map[string]uint64, error) {
			return map[string]uint64{mailboxID: 0}, nil
		},
		FetchEnvelopesFunc: func(ctx context.Context, mID string, fromSeq uint64, limit int) ([]*types.Envelope, error) {
			return []*types.Envelope{
				{EnvelopeID: "env-1", MailboxID: mailboxID, Seq: 1, QoS: types.QoSCommand, Type: "cmd"},
			}, nil
		},
	}

	stream := &mockStream{}
	settings := newTestSettings()
	settings.FlowControl.MaxInflightTotal = 0 // artificially block all delivery

	session := types.NewSession(serverID, "org-1", 1, "fp", nil, stream)
	session.Subscriptions = []string{mailboxID}

	svc := NewDeliveryService(repo, nil, settings, testMetrics).(*deliveryServiceImpl)
	deliveredUpTo := make(map[string]uint64)

	svc.deliverFromMailbox(context.Background(), session, mailboxID, deliveredUpTo)

	assert.Len(t, stream.sent, 0, "no delivery when flow control limit is 0")
	assert.Equal(t, uint64(0), deliveredUpTo[mailboxID], "deliveredUpTo should not advance")
}

func TestDeliverFromMailbox_BatchAdvancesDeliveredUpTo(t *testing.T) {
	// When a batch of envelopes is delivered, deliveredUpTo should advance to the highest seq.
	mailboxID := "mbox-1"
	serverID := "server-1"

	repo := &MockRepository{
		GetAckPositionsFunc: func(ctx context.Context, sid string) (map[string]uint64, error) {
			return map[string]uint64{mailboxID: 0}, nil
		},
		FetchEnvelopesFunc: func(ctx context.Context, mID string, fromSeq uint64, limit int) ([]*types.Envelope, error) {
			return []*types.Envelope{
				{EnvelopeID: "env-1", MailboxID: mailboxID, Seq: 1, QoS: types.QoSCommand, Type: "cmd"},
				{EnvelopeID: "env-2", MailboxID: mailboxID, Seq: 2, QoS: types.QoSCommand, Type: "cmd"},
				{EnvelopeID: "env-3", MailboxID: mailboxID, Seq: 3, QoS: types.QoSCommand, Type: "cmd"},
			}, nil
		},
	}

	stream := &mockStream{}
	session := types.NewSession(serverID, "org-1", 1, "fp", nil, stream)
	session.Subscriptions = []string{mailboxID}

	svc := NewDeliveryService(repo, nil, newTestSettings(), testMetrics).(*deliveryServiceImpl)
	deliveredUpTo := make(map[string]uint64)

	svc.deliverFromMailbox(context.Background(), session, mailboxID, deliveredUpTo)

	assert.Equal(t, uint64(3), deliveredUpTo[mailboxID], "deliveredUpTo should advance to highest seq in batch")
	assert.Len(t, stream.sent, 1, "one DELIVER frame with 3 envelopes")
}

func TestDeliverFromMailbox_SendFailureDoesNotAdvanceCursor(t *testing.T) {
	// If DeliverToSession fails (stream error), deliveredUpTo should NOT advance.
	mailboxID := "mbox-1"
	serverID := "server-1"

	repo := &MockRepository{
		GetAckPositionsFunc: func(ctx context.Context, sid string) (map[string]uint64, error) {
			return map[string]uint64{mailboxID: 0}, nil
		},
		FetchEnvelopesFunc: func(ctx context.Context, mID string, fromSeq uint64, limit int) ([]*types.Envelope, error) {
			return []*types.Envelope{
				{EnvelopeID: "env-1", MailboxID: mailboxID, Seq: 1, QoS: types.QoSCommand, Type: "cmd"},
			}, nil
		},
	}

	stream := &mockStream{sendErr: assert.AnError}
	session := types.NewSession(serverID, "org-1", 1, "fp", nil, stream)
	session.Subscriptions = []string{mailboxID}

	svc := NewDeliveryService(repo, nil, newTestSettings(), testMetrics).(*deliveryServiceImpl)
	deliveredUpTo := make(map[string]uint64)

	svc.deliverFromMailbox(context.Background(), session, mailboxID, deliveredUpTo)

	assert.Equal(t, uint64(0), deliveredUpTo[mailboxID], "deliveredUpTo must not advance on send failure")
}
