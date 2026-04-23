package admin_server

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// toGRPCError wraps an error as a gRPC Internal status error.
// Callers that need a specific code should call status.Error directly.
func toGRPCError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := status.FromError(err); ok {
		return err
	}
	return status.Error(codes.Internal, err.Error())
}
