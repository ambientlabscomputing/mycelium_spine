Below is a tight protocol + semantics spec Underleaf Mycelium Spine (UMS) — RFC-style Spec

0. Scope

UMS is the cloud↔edge control fabric protocol that replaces “Event Bus v4” semantics for command/control and targeted messaging. It provides:
	•	long-lived client-initiated connectivity (NAT safe)
	•	server push over a single stream (snappy UX)
	•	durable mailboxes for offline delivery
	•	QoS classes and backpressure behavior
	•	explicit ack + retry semantics
	•	cluster-level fanout without “poll fetch” loops

UMS is not Kafka. It is a connection-oriented control fabric.

⸻

1. Entities and Terms

Agent: edge-side process that dials out (typically UA-managed Link daemon).

Link: the edge-side network component that maintains the stream.

Control Plane: cloud service that terminates streams and manages mailboxes.

Mailbox: durable, per-target queue owned by the Control Plane.

Envelope: a single message unit delivered over stream and stored in mailboxes.

Target: routing identity for a mailbox (e.g., server/cluster/org).

⸻

2. Transport

UMS runs over one of:
	•	gRPC bidirectional streaming (preferred) over HTTP/2
	•	fallback: WebSocket streaming with identical frame semantics (optional, V2)

This spec defines semantic frames, independent of transport.

Transport invariants
	•	The agent initiates the connection (outbound-only).
	•	The connection is long-lived; the server pushes frames immediately.
	•	The agent must support reconnect with session resumption.

⸻

3. Identity and Session

3.1 Session establishment

Client opens stream and sends HELLO as first frame.

HELLO fields (required):
	•	protocol_version
	•	server_id (stable Underleaf identity)
	•	org_id (if applicable)
	•	device_fingerprint (TPM/SW attestation hash; optional but recommended)
	•	capabilities_summary_hash (optional; for future optimization)
	•	resume_token (optional; for session resumption)
	•	client_features (bitset: supports_compression, supports_batch_ack, supports_qos_drop, etc.)

Server replies with WELCOME.

WELCOME fields:
	•	session_id (ephemeral)
	•	session_epoch (monotonic per server_id)
	•	resume_token (rotating)
	•	server_time_ms
	•	policy (limits: max_inflight_by_qos, max_batch_bytes, heartbeat_interval)

3.2 Session invariants
	•	A server_id may have at most one active session per “role” (see §11 clusters). Competing sessions trigger deterministic behavior (kick old vs reject new).
	•	Every envelope delivered has a monotonically increasing seq per mailbox.
	•	Server may rotate resume tokens; client must persist latest.

⸻

4. Routing and Mailboxes

4.1 Target types

UMS supports these first-class mailbox targets:
	•	SERVER(server_id)
	•	CLUSTER(cluster_id)
	•	ORG(org_id)
	•	SERVICE(service_id) (optional, future)
	•	BROADCAST(scope) (restricted, admin-only)

Mailboxes are strictly isolated by Organization ID. The internal routing key for any mailbox is a composite of `(target_type, target_id, org_id)`. This ensures that even if a UUID collision occurs, messages cannot cross organizational boundaries. Publishers must provide the correct `org_id` context when publishing, and subscribers are only granted access to mailboxes matching their authenticated `org_id`.

4.2 Mailbox model

A mailbox is a durable ordered log with:
	•	mailbox_id
	•	ordered envelopes (seq=1..N)
	•	retention policy (time + max bytes + max envelopes)
	•	per-QoS partitioning (logical queues)

Critical requirement: server does not run “scan segment + filter” to decide delivery. Publishing must resolve targets and append directly to mailbox(es).

4.3 Publish resolution

Publishers (Server API, UCRS, etc.) call the Control Plane:

Publish(envelope, targets[])

Control Plane:
	1.	validates authz
	2.	resolves targets → mailbox_id[]
	3.	appends envelope to each mailbox with per-mailbox seq
	4.	if target session is active, pushes immediately (subject to flow control)

⸻

5. Envelope format

Each delivered message is an Envelope:

Core fields:
	•	envelope_id (UUID or ULID)
	•	mailbox_id
	•	seq (uint64, monotonic per mailbox)
	•	qos (enum)
	•	type (string or enum; e.g. command.run, deploy.apply, config.delta)
	•	created_at_ms
	•	expires_at_ms (optional but recommended for commands)
	•	trace_id (optional)
	•	payload (bytes; JSON/protobuf/CBOR allowed)

Optional routing hints:
	•	dedupe_key (for idempotency)
	•	priority (within QoS)
	•	requires_ack (default true for COMMAND/CONTROL, false for TELEMETRY)

⸻

6. QoS classes (and what they mean)

UMS defines QoS to enforce backpressure policies:

QoS = COMMAND
	•	delivery: at-least-once
	•	requires ack: yes
	•	persistence: durable mailbox
	•	drop policy: never drop unless expired
	•	expiry: recommended short TTL (e.g., minutes)

QoS = CONTROL
	•	delivery: at-least-once
	•	requires ack: yes
	•	persistence: durable mailbox (short retention)
	•	drop policy: drop if superseded (optional “collapse keys”)

QoS = TELEMETRY
	•	delivery: best-effort (at-most-once acceptable)
	•	requires ack: default no (can be yes for rare metrics)
	•	persistence: optional mailbox (often in-memory buffer only)
	•	drop policy: drop-first under backpressure

Invariant: Under backpressure, the system must sacrifice TELEMETRY before COMMAND.

⸻

7. Acknowledgements and delivery semantics

7.1 Ack modes

Two ack styles:

(A) Cumulative ACK (preferred):
	•	ACK(mailbox_id, seq_acked)
	•	implies all seq ≤ seq_acked are processed and committed

(B) Selective ACK (optional):
	•	ACK_SET(mailbox_id, seqs[])
	•	only if needed for out-of-order processing

Default is cumulative ACK.

7.2 Processing contract (at-least-once)

For COMMAND/CONTROL:
	•	Server may redeliver any unacked envelopes after reconnect.
	•	Client must implement idempotency using envelope_id and/or dedupe_key.

7.3 Exactly-once (explicitly out of scope)

UMS does not promise exactly-once. It promises:
	•	ordered per-mailbox delivery
	•	at-least-once for COMMAND/CONTROL
	•	best-effort for TELEMETRY

⸻

8. Flow control and backpressure

8.1 Inflight limits

Server enforces per-session:
	•	max_inflight_total
	•	max_inflight_by_qos[qos]
	•	max_batch_bytes

Server must not exceed these without receiving ACKs.

8.2 Batching

Server may send envelopes in batches:

DELIVER(batch_id, envelopes[])

Client ACKs cumulatively per mailbox, not per batch.

8.3 Backpressure behavior

If client falls behind:
	1.	TELEMETRY is dropped (or downsampled)
	2.	CONTROL may be collapsed by key (if configured)
	3.	COMMAND is retained and delivered when possible unless expired

Client may send FLOW_HINT:
	•	“I am overloaded; reduce telemetry rate”
	•	“Pause CONTROL except critical”
	•	“Increase heartbeat interval”

⸻

9. Reconnect and resume

9.1 Heartbeats

Both sides send PING/PONG at negotiated interval.
Missed heartbeats trigger reconnect.

**Timestamp Convention:**
To ensure cross-language compatibility and consistent database querying, all session timestamps (e.g., `connected_at`, `last_heartbeat`) persisted in the database MUST use the RFC3339 string format (e.g., `"2026-02-26T15:04:05Z"`). However, at the protobuf/gRPC boundary (e.g., `server_time_ms` in the `WelcomeFrame`), timestamps are transmitted as `int64` Unix milliseconds for efficiency.

9.2 Resume

Client reconnects and sends HELLO with resume_token.

Server resumes session if:
	•	token valid
	•	session_epoch matches expected or can be advanced safely

Server responds with RESUME_OK including:
	•	last server-known ack per mailbox for this server
	•	any policy changes

If resume fails:
	•	server returns RESUME_DENIED
	•	client performs full reconnect and mailbox replay begins from last committed acks

9.3 Replay rules

Upon reconnect, server must deliver from:
	•	last_acked_seq + 1 for each subscribed mailbox
	•	respecting ordering per mailbox

⸻

10. Subscription model (what mailboxes does a server receive?)

On connect, client sends SUBSCRIBE frames.

Examples:
	•	always subscribe to SERVER(server_id) mailbox
	•	subscribe to CLUSTER(cluster_id) if in cluster
	•	optionally subscribe to ORG(org_id) for org-wide notices

SUBSCRIBE fields:
	•	list of targets
	•	optional filters (discouraged; prefer separate mailbox types)
	•	QoS preferences (telemetry disabled, etc.)

Server replies SUBSCRIBE_OK with resolved mailbox_ids.

Invariant: subscription changes take effect immediately without reconnect.

⸻

11. Cluster fanout (without polling)

To send to “all servers in cluster”:
	•	publish to CLUSTER(cluster_id) mailbox
	•	each server subscribed receives the envelope over its open stream

For “one leader only” semantics:
	•	publish to CLUSTER(cluster_id) with deliver_role=LEADER (optional field)
	•	server resolves to the current leader’s SERVER mailbox (requires leader mapping from UA heartbeats)

This preserves immediacy and avoids scanning/filtering.

⸻

12. Ordering guarantees

UMS guarantees:
	•	Total order per mailbox (by seq)
	•	No global total order across mailboxes
	•	If a publisher needs ordered multi-target delivery, it must publish to a shared mailbox (e.g., cluster mailbox) or include causal metadata.

⸻

13. Idempotency and deduplication

Client must handle duplicates for COMMAND/CONTROL.

Required behaviors:
	•	Maintain an LRU of recently seen envelope_id per mailbox (time-bounded)
	•	For “apply desired state” style commands, use dedupe_key = desired_state_hash so replays collapse naturally
	•	For “run arbitrary command” style, use envelope_id for idempotency tracking and report status with the same ID

⸻

14. Error handling

Server → client
	•	ERROR(code, message, retryable, context)
Used for:
	•	auth failures
	•	protocol violations
	•	mailbox unavailable
	•	rate limits

Client → server
	•	NACK(mailbox_id, seq, reason) (optional)
Used when:
	•	message is malformed
	•	client cannot process due to permanent constraints

Policy: NACK does not delete; it triggers either dead-lettering or quarantine by server policy.

⸻

15. Operational requirements

Metrics (minimum)
	•	session_count, reconnect_rate
	•	p50/p95 delivery latency (publish→deliver)
	•	inflight by QoS
	•	backlog per mailbox
	•	ack lag per mailbox
	•	dropped telemetry counts
	•	expired commands count

SLO targets (suggested)
	•	publish→deliver p95 < 200ms (within region)
	•	reconnect recovery < 2s median
	•	command delivery success > 99.9% (excluding offline servers)

⸻
