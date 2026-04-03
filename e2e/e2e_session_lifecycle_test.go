package e2e

import (
	"testing"
	"time"

	umsv1 "github.com/ambientlabscomputing/mycelium_spine/proto/ums/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSessionLifecycle_HelloAndWelcome tests the basic session establishment
func TestSessionLifecycle_HelloAndWelcome(t *testing.T) {
if testing.Short() {
t.Skip("Skipping e2e test in short mode")
}

env := NewTestEnv(t)
defer env.Cleanup()

// Create and connect client
client := env.CreateClient("test-server-01", "test-org")
defer client.Close()

err := client.Connect(env.Ctx)
require.NoError(t, err, "Failed to connect client")

// Verify session was established
sessionID := client.SessionID()
assert.NotEmpty(t, sessionID, "Session ID should be set")

resumeToken := client.ResumeToken()
assert.NotEmpty(t, resumeToken, "Resume token should be set")

env.Logger.Info("Session established",
"session_id", sessionID,
"resume_token", resumeToken)
}

// TestSessionLifecycle_ReconnectWithResume tests reconnection using a resume token
func TestSessionLifecycle_ReconnectWithResume(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	env := NewTestEnv(t)
	defer env.Cleanup()

	// Create and connect first client
	client1 := env.CreateClient("test-server-01", "test-org")
	defer client1.Close()

	err := client1.Connect(env.Ctx)
	require.NoError(t, err, "Failed to connect client")

	originalSessionID := client1.SessionID()
	resumeToken := client1.ResumeToken()

	env.Logger.Info("First connection established",
		"session_id", originalSessionID,
		"resume_token", resumeToken)

	// Subscribe to a target
	err = client1.Subscribe([]*umsv1.Target{CreateServerTarget("test-server-01")})
	require.NoError(t, err, "Failed to subscribe")

	time.Sleep(100 * time.Millisecond)

	// Disconnect first client
	client1.Close()
	time.Sleep(100 * time.Millisecond)

	// Reconnect with resume token
	client2 := env.CreateClientWithResume("test-server-01", "test-org", resumeToken)
	defer client2.Close()

	err = client2.Connect(env.Ctx)
	require.NoError(t, err, "Failed to reconnect with resume token")

	// Session should be resumed
	newSessionID := client2.SessionID()
	assert.Equal(t, originalSessionID, newSessionID, "Session ID should be the same after resume")

	env.Logger.Info("Session resumed successfully", "session_id", newSessionID)
}
