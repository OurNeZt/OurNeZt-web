package core

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/OurNeZt/ournezt-web/internal/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type requestFailureKey struct{}
type requestFailure struct{ code atomic.Int32 }

// WithRequestTimeout shares one Core time budget across session validation and
// all business RPCs in an HTTP request. Earlier deadlines and cancellation survive.
func WithRequestTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, positiveTimeout(timeout))
	return context.WithValue(ctx, requestFailureKey{}, &requestFailure{}), cancel
}

// RequestFailure lets page rendering report dependency failures even when an
// optional RPC's error was otherwise ignored, rather than show incomplete data.
func RequestFailure(ctx context.Context) codes.Code {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return codes.DeadlineExceeded
	}
	if failure, ok := ctx.Value(requestFailureKey{}).(*requestFailure); ok {
		return codes.Code(failure.code.Load())
	}
	return codes.OK
}

func positiveTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return config.DefaultRequestTimeout
	}
	return timeout
}

// rpcDeadline bounds every unary call on the shared Core connection, including
// calls made without the Web request middleware. Cancellation releases the timer.
func rpcDeadline(timeout time.Duration) grpc.UnaryClientInterceptor {
	timeout = positiveTimeout(timeout)
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		var err error
		if ctx.Err() != nil {
			err = status.FromContextError(ctx.Err()).Err()
		} else {
			err = invoker(ctx, method, req, reply, cc, opts...)
		}
		code := status.Code(err)
		if code == codes.DeadlineExceeded || code == codes.Unavailable {
			if failure, ok := ctx.Value(requestFailureKey{}).(*requestFailure); ok {
				failure.code.Store(int32(code))
			}
		}
		return err
	}
}
