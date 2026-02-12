module github.com/ambientlabscomputing/mycelium_spine/sdk

go 1.24

require (
	github.com/ambientlabscomputing/mycelium_spine/proto v0.0.0
	google.golang.org/grpc v1.78.0
	google.golang.org/protobuf v1.36.6
)

require (
	golang.org/x/net v0.37.0 // indirect
	golang.org/x/sys v0.29.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250127172529-29210b9bc287 // indirect
)

replace github.com/ambientlabscomputing/mycelium_spine/proto => ../proto
