// Package client provides a gRPC admin client for mspinectl.
package client

import (
	"google.golang.org/grpc"

	"github.com/ambientlabscomputing/mycelium_spine/proto/admin"
)

// AdminClient wraps a gRPC connection and provides lazy service client accessors.
type AdminClient struct {
	conn      *grpc.ClientConn
	health    admin.AdminHealthServiceClient
	sessions  admin.AdminSessionsServiceClient
	mailboxes admin.AdminMailboxesServiceClient
	cursors   admin.AdminCursorsServiceClient
	workers   admin.AdminWorkersServiceClient
}

// NewAdminClientFromConn creates an AdminClient from an existing gRPC connection.
func NewAdminClientFromConn(conn *grpc.ClientConn) *AdminClient {
	return &AdminClient{conn: conn}
}

func (c *AdminClient) Health() admin.AdminHealthServiceClient {
	if c.health == nil {
		c.health = admin.NewAdminHealthServiceClient(c.conn)
	}
	return c.health
}

func (c *AdminClient) Sessions() admin.AdminSessionsServiceClient {
	if c.sessions == nil {
		c.sessions = admin.NewAdminSessionsServiceClient(c.conn)
	}
	return c.sessions
}

func (c *AdminClient) Mailboxes() admin.AdminMailboxesServiceClient {
	if c.mailboxes == nil {
		c.mailboxes = admin.NewAdminMailboxesServiceClient(c.conn)
	}
	return c.mailboxes
}

func (c *AdminClient) Cursors() admin.AdminCursorsServiceClient {
	if c.cursors == nil {
		c.cursors = admin.NewAdminCursorsServiceClient(c.conn)
	}
	return c.cursors
}

func (c *AdminClient) Workers() admin.AdminWorkersServiceClient {
	if c.workers == nil {
		c.workers = admin.NewAdminWorkersServiceClient(c.conn)
	}
	return c.workers
}
