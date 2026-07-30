// Spike s64-bri-grpc — HONEST feasibility probe for bri.grpc.
//
// The heavy question: can cljgo offer gRPC WITHOUT the user needing an
// external protoc toolchain?
//
// This driver proves the NO-CODEGEN path end to end:
//   1. A .proto source lives as a Go string (could be a file, a REPL form).
//   2. github.com/bufbuild/protocompile parses+links it IN-PROCESS — no protoc
//      binary, no C compiler, no protoc-gen-go. Pure Go.
//   3. From the resulting descriptors we register a gRPC service at RUNTIME
//      using grpc.ServiceDesc + a generic handler and google.golang.org/
//      protobuf/types/dynamicpb messages (no generated .pb.go types at all).
//   4. A real gRPC client dials over TCP and performs one unary RPC round-trip,
//      also using dynamicpb for request/response.
//
// If this runs, cljgo can define+serve+call gRPC services from a .proto with
// zero external tooling. That is the key result.
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/bufbuild/protocompile"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

const protoSrc = `
syntax = "proto3";
package demo;

message HelloRequest {
  string name = 1;
}
message HelloReply {
  string message = 1;
}
service Greeter {
  rpc SayHello (HelloRequest) returns (HelloReply);
}
`

// compileInProcess parses the .proto string with protocompile — no protoc.
func compileInProcess(ctx context.Context) (protoreflect.FileDescriptor, error) {
	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			Accessor: protocompile.SourceAccessorFromMap(map[string]string{
				"demo.proto": protoSrc,
			}),
		}),
		SourceInfoMode: protocompile.SourceInfoStandard,
	}
	files, err := compiler.Compile(ctx, "demo.proto")
	if err != nil {
		return nil, err
	}
	return files[0], nil
}

func main() {
	ctx := context.Background()

	// --- 1 & 2: pure-Go, in-process proto compilation (no protoc) ----------
	fd, err := compileInProcess(ctx)
	if err != nil {
		log.Fatalf("protocompile failed: %v", err)
	}
	svc := fd.Services().Get(0)
	method := svc.Methods().Get(0)
	inDesc := method.Input()
	outDesc := method.Output()
	fullMethod := fmt.Sprintf("/%s/%s", svc.FullName(), method.Name())
	fmt.Printf("compiled in-process (no protoc): service=%s method=%s\n", svc.FullName(), method.Name())
	fmt.Printf("  input=%s output=%s rpc=%s\n", inDesc.FullName(), outDesc.FullName(), fullMethod)

	// Register the file's messages so the proto codec can resolve them if needed.
	// (Not strictly required for dynamicpb round-trips, but realistic.)
	_ = protoregistry.GlobalFiles

	// --- 3: register a gRPC service at RUNTIME with a generic handler ------
	srv := grpc.NewServer()
	handlerFD := fd // capture

	handler := func(_ interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
		req := dynamicpb.NewMessage(handlerFD.Services().Get(0).Methods().Get(0).Input())
		if err := dec(req); err != nil {
			return nil, err
		}
		name := req.Get(req.Descriptor().Fields().ByName("name")).String()
		resp := dynamicpb.NewMessage(handlerFD.Services().Get(0).Methods().Get(0).Output())
		resp.Set(resp.Descriptor().Fields().ByName("message"), protoreflect.ValueOfString("hello, "+name+"!"))
		if interceptor != nil {
			return interceptor(ctx, req, &grpc.UnaryServerInfo{FullMethod: fullMethod}, func(ctx context.Context, r interface{}) (interface{}, error) {
				return resp, nil
			})
		}
		return resp, nil
	}

	sd := &grpc.ServiceDesc{
		ServiceName: string(svc.FullName()),
		HandlerType: (*interface{})(nil),
		Methods: []grpc.MethodDesc{
			{MethodName: string(method.Name()), Handler: handler},
		},
		Streams:  []grpc.StreamDesc{},
		Metadata: "demo.proto",
	}
	srv.RegisterService(sd, nil)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	addr := lis.Addr().String()
	go func() {
		if err := srv.Serve(lis); err != nil {
			log.Printf("serve stopped: %v", err)
		}
	}()
	fmt.Printf("gRPC server up at %s (service registered from runtime descriptor)\n", addr)

	// --- 4: real gRPC client, dynamicpb request/response, unary round-trip -
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	reqMsg := dynamicpb.NewMessage(inDesc)
	reqMsg.Set(inDesc.Fields().ByName("name"), protoreflect.ValueOfString("cljgo"))
	respMsg := dynamicpb.NewMessage(outDesc)

	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := conn.Invoke(cctx, fullMethod, reqMsg, respMsg); err != nil {
		log.Fatalf("invoke: %v", err)
	}

	got := respMsg.Get(outDesc.Fields().ByName("message")).String()
	fmt.Printf("client sent name=%q\n", "cljgo")
	fmt.Printf("server replied message=%q\n", got)
	if got != "hello, cljgo!" {
		log.Fatalf("ROUND-TRIP MISMATCH: got %q", got)
	}
	fmt.Println("ROUND-TRIP OK: unary gRPC via protocompile + dynamicpb, NO protoc, NO codegen")

	srv.GracefulStop()
}
