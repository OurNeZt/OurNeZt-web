package web

import (
	"context"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OurNeZt/ournezt-web/internal/config"
	"github.com/OurNeZt/ournezt-web/internal/core"
	pb "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type rpcRecorder struct {
	t               *testing.T
	deadlines       []time.Time
	sessionContexts []context.Context
}

func (r *rpcRecorder) observe(ctx context.Context, authenticated bool) {
	r.t.Helper()
	deadline, ok := ctx.Deadline()
	if !ok {
		r.t.Fatal("Core call has no deadline")
	}
	r.deadlines = append(r.deadlines, deadline)
	if authenticated {
		md, _ := metadata.FromOutgoingContext(ctx)
		if len(md.Get("x-session-token")) != 1 || md.Get("x-session-token")[0] != "token" {
			r.t.Error("session token missing from business RPC")
		}
	}
}

func (r *rpcRecorder) stall(ctx context.Context, authenticated bool) error {
	r.observe(ctx, authenticated)
	select {
	case <-ctx.Done():
		return status.Error(codes.DeadlineExceeded, "private Core timeout details")
	case <-time.After(time.Second):
		return status.Error(codes.Internal, "test RPC was not canceled")
	}
}

type deadlineAuthClient struct {
	pb.AuthServiceClient
	recorder       *rpcRecorder
	role           string
	sessionFailure codes.Code
}

func (f *deadlineAuthClient) ValidateSession(ctx context.Context, _ *pb.ValidateSessionRequest, _ ...grpc.CallOption) (*pb.ValidateSessionResponse, error) {
	if f.sessionFailure == codes.DeadlineExceeded {
		return nil, f.recorder.stall(ctx, false)
	}
	f.recorder.observe(ctx, false)
	f.recorder.sessionContexts = append(f.recorder.sessionContexts, ctx)
	if f.sessionFailure != codes.OK {
		return nil, status.Error(f.sessionFailure, "private connection details")
	}
	return &pb.ValidateSessionResponse{User: &pb.User{Id: "user", Role: f.role}}, nil
}
func (f *deadlineAuthClient) Login(ctx context.Context, _ *pb.LoginRequest, _ ...grpc.CallOption) (*pb.LoginResponse, error) {
	return nil, f.recorder.stall(ctx, false)
}
func (f *deadlineAuthClient) ListUsers(ctx context.Context, _ *pb.ListUsersRequest, _ ...grpc.CallOption) (*pb.ListUsersResponse, error) {
	return nil, f.recorder.stall(ctx, true)
}
func (f *deadlineAuthClient) UpdateMyAccount(ctx context.Context, _ *pb.UpdateMyAccountRequest, _ ...grpc.CallOption) (*pb.User, error) {
	return nil, f.recorder.stall(ctx, true)
}

type deadlineFamilyClient struct {
	pb.FamilyServiceClient
	recorder *rpcRecorder
	delay    time.Duration
}

func (f *deadlineFamilyClient) ListUserFamilies(ctx context.Context, _ *pb.ListUserFamiliesRequest, _ ...grpc.CallOption) (*pb.ListUserFamiliesResponse, error) {
	if f.delay > 0 {
		f.recorder.observe(ctx, true)
		select {
		case <-time.After(f.delay):
			return &pb.ListUserFamiliesResponse{Families: []*pb.Family{{Id: "family"}}}, nil
		case <-ctx.Done():
			return nil, status.FromContextError(ctx.Err()).Err()
		}
	}
	return nil, f.recorder.stall(ctx, true)
}

type deadlineDashboardClient struct {
	pb.DashboardServiceClient
	recorder *rpcRecorder
}

func (f *deadlineDashboardClient) GetHouseholdDashboard(ctx context.Context, _ *pb.GetHouseholdDashboardRequest, _ ...grpc.CallOption) (*pb.HouseholdDashboard, error) {
	return nil, f.recorder.stall(ctx, true)
}

type deadlinePersonClient struct {
	pb.PersonServiceClient
	recorder *rpcRecorder
}

func (f *deadlinePersonClient) GetPersonProfile(ctx context.Context, _ *pb.GetPersonProfileRequest, _ ...grpc.CallOption) (*pb.PersonProfile, error) {
	return nil, f.recorder.stall(ctx, true)
}

type deadlineHousingClient struct {
	pb.HousingServiceClient
	recorder *rpcRecorder
}

func (f *deadlineHousingClient) GetHousingOption(ctx context.Context, _ *pb.GetHousingOptionRequest, _ ...grpc.CallOption) (*pb.HousingOption, error) {
	return nil, f.recorder.stall(ctx, true)
}

func TestCoreDeadlinesAcrossBusinessRoutes(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	for _, tc := range []struct {
		method, path, role, body string
		code                     int
	}{
		{"GET", "/families", "user", "", http.StatusGatewayTimeout},
		{"GET", "/dashboard", "user", "", http.StatusFound},
		{"GET", "/people/person/edit", "user", "", http.StatusFound},
		{"GET", "/housing/home", "user", "", http.StatusFound},
		{"GET", "/profile", "user", "", http.StatusGatewayTimeout},
		{"GET", "/admin/users", "admin", "", http.StatusFound},
		{"POST", "/profile/account", "user", "email=a%40example.com&display_name=User", http.StatusFound},
		{"POST", "/login", "", "email=a%40example.com&password=unused", http.StatusFound},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			recorder := &rpcRecorder{t: t}
			clients := &core.Clients{
				Auth:   &deadlineAuthClient{recorder: recorder, role: tc.role},
				Family: &deadlineFamilyClient{recorder: recorder}, Person: &deadlinePersonClient{recorder: recorder}, Housing: &deadlineHousingClient{recorder: recorder},
			}
			router, err := NewRouter(config.Config{RequestTimeout: 30 * time.Millisecond, SessionCookieName: "session"}, clients)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.method == http.MethodPost {
				addCSRF(router, req)
			}
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}
			if tc.role != "" {
				req.AddCookie(&http.Cookie{Name: "session", Value: "token"})
			}
			w := httptest.NewRecorder()
			started := time.Now()
			router.ServeHTTP(w, req)
			visible := w.Body.String() + w.Header().Get("Location")
			if w.Code != tc.code || time.Since(started) > time.Second || strings.Contains(visible, "private") {
				t.Fatalf("unsafe or unbounded response: %d %s", w.Code, visible)
			}
			expectedCalls := 2
			if tc.role == "" {
				expectedCalls = 1
			}
			if len(recorder.deadlines) < expectedCalls {
				t.Fatalf("route did not exercise its business RPC: %d calls", len(recorder.deadlines))
			}
			for _, deadline := range recorder.deadlines {
				if !deadline.Equal(recorder.deadlines[0]) {
					t.Error("RPCs did not share the request deadline")
				}
			}
			for _, ctx := range recorder.sessionContexts {
				if ctx.Err() == nil {
					t.Error("completed request context was not canceled")
				}
			}
		})
	}
}

func TestCoreSequentialRPCsShareOneBudget(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	recorder := &rpcRecorder{t: t}
	clients := &core.Clients{Auth: &deadlineAuthClient{recorder: recorder, role: "user"}, Family: &deadlineFamilyClient{recorder: recorder, delay: 40 * time.Millisecond}, Dashboard: &deadlineDashboardClient{recorder: recorder}}
	router, err := NewRouter(config.Config{RequestTimeout: 80 * time.Millisecond, SessionCookieName: "session"}, clients)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "token"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if len(recorder.deadlines) != 3 {
		t.Fatalf("expected session, family, dashboard RPCs: %d", len(recorder.deadlines))
	}
	for _, deadline := range recorder.deadlines {
		if !deadline.Equal(recorder.deadlines[0]) {
			t.Fatal("sequential RPC received a fresh timeout budget")
		}
	}
	location, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusFound || location.Query().Get("error") != coreTimeoutMessage {
		t.Fatalf("timeout was not shown safely: %d %s", w.Code, location)
	}
}

func TestCoreSessionFailurePreservesCookie(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	for _, code := range []codes.Code{codes.DeadlineExceeded, codes.Unavailable} {
		recorder := &rpcRecorder{t: t}
		router, err := NewRouter(config.Config{RequestTimeout: 20 * time.Millisecond, SessionCookieName: "session"}, &core.Clients{Auth: &deadlineAuthClient{recorder: recorder, role: "user", sessionFailure: code}})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("GET", "/families", nil)
		addCSRF(router, req)
		req.AddCookie(&http.Cookie{Name: "session", Value: "token"})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		expected := http.StatusServiceUnavailable
		if code == codes.DeadlineExceeded {
			expected = http.StatusGatewayTimeout
		}
		if w.Code != expected || w.Header().Get("Set-Cookie") != "" || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "private") || !strings.Contains(w.Body.String(), "</html>") {
			t.Fatalf("session failure response is unsafe: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestCoreEarlierRequestDeadlineIsRespected(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	recorder := &rpcRecorder{t: t}
	router, err := NewRouter(config.Config{RequestTimeout: time.Second, SessionCookieName: "session"}, &core.Clients{Auth: &deadlineAuthClient{recorder: recorder, role: "user"}, Family: &deadlineFamilyClient{recorder: recorder}})
	if err != nil {
		t.Fatal(err)
	}
	parent, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest("GET", "/families", nil).WithContext(parent)
	req.AddCookie(&http.Cookie{Name: "session", Value: "token"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	expected, _ := parent.Deadline()
	for _, deadline := range recorder.deadlines {
		if !deadline.Equal(expected) {
			t.Fatal("earlier HTTP deadline was extended")
		}
	}
	if w.Code != http.StatusGatewayTimeout {
		t.Fatalf("expired parent was not handled: %d", w.Code)
	}
}

func TestCoreUnavailableCannotRenderEmptyFamilyData(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var outage atomic.Bool
	outage.Store(true)
	server := grpc.NewServer(grpc.UnknownServiceHandler(func(_ any, stream grpc.ServerStream) error {
		if err := stream.RecvMsg(&pb.LoginRequest{}); err != nil {
			return err
		}
		method, _ := grpc.MethodFromServerStream(stream)
		if strings.HasSuffix(method, "/ValidateSession") {
			return stream.SendMsg(&pb.ValidateSessionResponse{User: &pb.User{Id: "user", Role: "user"}})
		}
		if !outage.Load() {
			return stream.SendMsg(&pb.ListUserFamiliesResponse{})
		}
		return status.Error(codes.Unavailable, "private Core address and connection details")
	}))
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()
	cfg := config.Config{CoreGRPCAddr: listener.Addr().String(), RequestTimeout: time.Second, SessionCookieName: "session"}
	clients, err := core.NewClients(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = clients.Close() }()
	router, err := NewRouter(cfg, clients)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/families", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "token"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), coreUnavailableMessage) || strings.Contains(w.Body.String(), "private Core") || strings.Contains(w.Body.String(), "No families") {
		t.Fatalf("Core failure rendered as empty data: %d %s", w.Code, w.Body.String())
	}
	outage.Store(false)
	req = httptest.NewRequest("GET", "/families", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "token"})
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "No families") {
		t.Fatalf("dependency failure leaked into the next request: %d", w.Code)
	}
}

func TestCoreErrorMessagesAreSafe(t *testing.T) {
	for _, tc := range []struct {
		err     error
		message string
	}{
		{context.DeadlineExceeded, coreTimeoutMessage},
		{fmt.Errorf("private detail: %w", context.DeadlineExceeded), coreTimeoutMessage},
		{status.Error(codes.DeadlineExceeded, "private Core detail"), coreTimeoutMessage},
		{status.Error(codes.Unavailable, "private Core address"), coreUnavailableMessage},
		{context.Canceled, coreUnavailableMessage},
		{status.Error(codes.InvalidArgument, "Family name is required"), "Family name is required"},
	} {
		if got := grpcMessage(tc.err); got != tc.message {
			t.Errorf("unsafe error message %q", got)
		}
	}
}

func (f *deadlineHousingClient) UpdateHousingNotes(ctx context.Context, _ *pb.UpdateHousingNotesRequest, _ ...grpc.CallOption) (*pb.UpdateHousingNotesResponse, error) {
	return nil, f.recorder.stall(ctx, true)
}
func (f *deadlineHousingClient) SaveHousingAnswer(ctx context.Context, _ *pb.HousingAnswer, _ ...grpc.CallOption) (*pb.HousingAnswer, error) {
	return nil, f.recorder.stall(ctx, true)
}
func (f *deadlineHousingClient) SaveHousingCriterion(ctx context.Context, _ *pb.HousingCriterion, _ ...grpc.CallOption) (*pb.HousingCriterion, error) {
	return nil, f.recorder.stall(ctx, true)
}
func (f *deadlineHousingClient) ListHousingCriteria(ctx context.Context, _ *pb.ListHousingCriteriaRequest, _ ...grpc.CallOption) (*pb.ListHousingCriteriaResponse, error) {
	return nil, f.recorder.stall(ctx, true)
}

func TestCoreTimeoutPreservesUnsavedHousingDrafts(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	draft := "Keep my unsaved <script>draft</script>"
	for _, tc := range []struct {
		path string
		form url.Values
	}{
		{"/housing/home/notes", url.Values{"notes": {draft}}},
		{"/housing/home/evaluation/criterion", url.Values{"state": {"complete"}, "rating": {"4"}, "notes": {draft}}},
		{"/housing/checklist", url.Values{"family_id": {"family"}, "name": {"Location"}, "display_order": {"1"}, "description": {draft}, "weight": {"2"}}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			recorder := &rpcRecorder{t: t}
			clients := &core.Clients{Auth: &deadlineAuthClient{recorder: recorder, role: "user"}, Housing: &deadlineHousingClient{recorder: recorder}}
			router, err := NewRouter(config.Config{RequestTimeout: 20 * time.Millisecond, SessionCookieName: "session"}, clients)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest("POST", tc.path, strings.NewReader(tc.form.Encode()))
			addCSRF(router, req)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.AddCookie(&http.Cookie{Name: "session", Value: "token"})
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			body := w.Body.String()
			if w.Code != http.StatusGatewayTimeout || !strings.Contains(body, html.EscapeString(draft)) || strings.Contains(body, "<script>draft</script>") || !strings.Contains(body, "</html>") {
				t.Fatalf("timeout lost or unsafely rendered a draft: %d %s", w.Code, body)
			}
			if tc.path == "/housing/home/evaluation/criterion" && !strings.Contains(body, "Rating: 4") {
				t.Error("unsaved rating was lost")
			}
			if tc.path == "/housing/checklist" && !strings.Contains(body, "Location") {
				t.Error("unsaved criterion name was lost")
			}
		})
	}
}
