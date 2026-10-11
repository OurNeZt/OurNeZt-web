package core

import (
	"context"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OurNeZt/ournezt-web/internal/config"
	pb "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestRPCDeadlinePreservesParentAndCancelsTimer(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	parent = metadata.AppendToOutgoingContext(parent, "x-session-token", "private-token")
	var invoked context.Context
	err := rpcDeadline(time.Second)(parent, "/test", nil, nil, nil, func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		invoked = ctx
		deadline, ok := ctx.Deadline()
		parentDeadline, _ := parent.Deadline()
		md, _ := metadata.FromOutgoingContext(ctx)
		if !ok || !deadline.Equal(parentDeadline) || len(md.Get("x-session-token")) != 1 || md.Get("x-session-token")[0] != "private-token" {
			t.Error("parent deadline or session metadata lost")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if invoked.Err() != context.Canceled {
		t.Fatal("completed RPC context was not canceled")
	}
	for _, timeout := range []time.Duration{0, -time.Second} {
		err := rpcDeadline(timeout)(context.Background(), "/test", nil, nil, nil, func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			deadline, ok := ctx.Deadline()
			remaining := time.Until(deadline)
			if !ok || remaining <= 4*time.Second || remaining > config.DefaultRequestTimeout {
				t.Errorf("unbounded fallback: %s", remaining)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestRPCDeadlineSkipsCanceledCalls(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := rpcDeadline(time.Second)(parent, "/test", nil, nil, nil, func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		called = true
		return nil
	})
	if called || status.Code(err) != codes.Canceled {
		t.Fatalf("canceled call was invoked: called=%t, err=%v", called, err)
	}
}

func TestRPCRequestFailureTracksIgnoredDependencyErrors(t *testing.T) {
	for _, code := range []codes.Code{codes.Unavailable, codes.DeadlineExceeded, codes.InvalidArgument} {
		ctx, cancel := WithRequestTimeout(context.Background(), time.Second)
		err := rpcDeadline(time.Second)(ctx, "/optional", nil, nil, nil, func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
			return status.Error(code, "private Core details")
		})
		expected := code
		if code == codes.InvalidArgument {
			expected = codes.OK
		}
		if status.Code(err) != code || RequestFailure(ctx) != expected {
			t.Errorf("dependency status was not recorded correctly: %v", err)
		}
		cancel()
	}
}

func stalledCore(t *testing.T) (string, *atomic.Int32) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	calls := &atomic.Int32{}
	server := grpc.NewServer(grpc.UnknownServiceHandler(func(_ any, stream grpc.ServerStream) error {
		// Consume the unary request so the test proves the server received the RPC.
		if err := stream.RecvMsg(&pb.LoginRequest{}); err != nil && err != io.EOF {
			return err
		}
		ctx := stream.Context()
		if _, ok := ctx.Deadline(); !ok {
			t.Error("Core received an RPC without a deadline")
		}
		md, _ := metadata.FromIncomingContext(ctx)
		if len(md.Get("x-session-token")) != 1 || md.Get("x-session-token")[0] != "reviewer-token" {
			t.Error("session metadata was not forwarded")
		}
		calls.Add(1)
		<-ctx.Done()
		return status.FromContextError(ctx.Err()).Err()
	}))
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	return listener.Addr().String(), calls
}

func TestRPCDeadlinesAgainstStalledCoreForEveryService(t *testing.T) {
	address, calls := stalledCore(t)
	clients, err := NewClients(context.Background(), config.Config{CoreGRPCAddr: address, RequestTimeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clients.Close() })
	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-session-token", "reviewer-token")
	checks := []struct {
		name   string
		invoke func(context.Context) error
	}{
		{"session", func(ctx context.Context) error {
			_, err := clients.Auth.ValidateSession(ctx, &pb.ValidateSessionRequest{})
			return err
		}},
		{"auth", func(ctx context.Context) error { _, err := clients.Auth.Login(ctx, &pb.LoginRequest{}); return err }},
		{"family", func(ctx context.Context) error {
			_, err := clients.Family.ListUserFamilies(ctx, &pb.ListUserFamiliesRequest{})
			return err
		}},
		{"person", func(ctx context.Context) error {
			_, err := clients.Person.ListPersonProfilesByFamily(ctx, &pb.ListPersonProfilesByFamilyRequest{})
			return err
		}},
		{"housing", func(ctx context.Context) error {
			_, err := clients.Housing.ListHousingOptions(ctx, &pb.ListHousingOptionsRequest{})
			return err
		}},
		{"income", func(ctx context.Context) error {
			_, err := clients.Income.CalculateHouseholdIncomeSummary(ctx, &pb.CalculateHouseholdIncomeSummaryRequest{})
			return err
		}},
		{"CPF", func(ctx context.Context) error {
			_, err := clients.CPF.CalculateCPFContribution(ctx, &pb.CalculateCPFContributionRequest{})
			return err
		}},
		{"dashboard", func(ctx context.Context) error {
			_, err := clients.Dashboard.GetHouseholdDashboard(ctx, &pb.GetHouseholdDashboardRequest{})
			return err
		}},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			started := time.Now()
			err := check.invoke(ctx)
			if status.Code(err) != codes.DeadlineExceeded || time.Since(started) > time.Second {
				t.Fatalf("stalled RPC was not bounded: %v (%s)", err, time.Since(started))
			}
		})
	}
	if calls.Load() != int32(len(checks)) {
		t.Fatalf("not every service reached Core: %d of %d", calls.Load(), len(checks))
	}
}

func TestRPCDeadlineBoundsUnavailableWaitForReady(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	clients, err := NewClients(context.Background(), config.Config{CoreGRPCAddr: address, RequestTimeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = clients.Close() }()
	started := time.Now()
	_, err = clients.Auth.Login(context.Background(), &pb.LoginRequest{}, grpc.WaitForReady(true))
	if status.Code(err) != codes.DeadlineExceeded || time.Since(started) > time.Second {
		t.Fatalf("unavailable Core was not bounded: %v", err)
	}
}
