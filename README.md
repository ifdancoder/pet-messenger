# Project pet-messenger

One Paragraph of project description goes here

## Getting Started

These instructions will get you a copy of the project up and running on your local machine for development and testing purposes. See deployment for notes on how to deploy the project on a live system.

## MakeFile

Run build make command with tests
```bash
make all
```

Build the application
```bash
make build
```

Run the application
```bash
make run
```
Create DB container
```bash
make docker-run
```

Shutdown DB Container
```bash
make docker-down
```

DB Integrations Test:
```bash
make itest
```

Live reload the application:
```bash
make watch
```

Run the test suite:
```bash
make test
```

Clean up binary from the last build:
```bash
make clean
```

## media-service (calls)

Group calls are served by a separate Pion SFU (`media-service`) that shares this
Go module. `api-service` drives the chosen media pod over a gRPC control plane
(`RoomControl`: `CreateRoom` / `CloseRoom` / `MuteParticipant` /
`RemoveParticipant` / `RoomStats`), defined in `proto/media/v1/media.proto`.

### Regenerating the gRPC code

Generated code lives in `internal/adapter/rpc/mediav1/` and is committed — there
is no codegen step in CI. Regenerate only when the `.proto` changes.

Install the toolchain (local plugins; no `protoc` needed):

```bash
go install github.com/bufbuild/buf/cmd/buf@latest
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
export PATH="$PATH:$(go env GOPATH)/bin"
```

Then from the repo root:

```bash
buf generate
```

`buf.gen.yaml` uses the **local** `protoc-gen-go` / `protoc-gen-go-grpc` plugins
(not the remote `buf.build/...` plugins) with `opt: [module=pet-messenger]`, so
the files land at `internal/adapter/rpc/mediav1/media.pb.go` and
`media_grpc.pb.go` per the proto's `go_package`.

### Running the call stack

`deploy/media/docker-compose.yml` brings up the full call stack (api-service,
media-service, Postgres, Redis, NATS, coturn). See `deploy/media/README.md`
for the step-by-step key generation, migration, and manual two-tab call
verification procedure (including forcing the TURN relay path).
