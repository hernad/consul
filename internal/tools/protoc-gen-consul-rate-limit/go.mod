module github.com/hernad/consul/internal/tools/protoc-gen-consul-rate-limit

go 1.19

replace github.com/hernad/consul/proto-public => ../../../proto-public

require (
	github.com/hernad/consul/proto-public v0.0.0-00010101000000-000000000000
	google.golang.org/protobuf v1.30.0
)

require github.com/google/go-cmp v0.5.9 // indirect
