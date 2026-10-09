package web

import (
	"context"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OurNeZt/ournezt-web/internal/core"
	pb "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type notesHousingClient struct {
	checklistHousingClient
	request *pb.UpdateHousingNotesRequest
	notes   string
	getErr  error
}

func (f *notesHousingClient) GetHousingOption(context.Context, *pb.GetHousingOptionRequest, ...grpc.CallOption) (*pb.HousingOption, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return &pb.HousingOption{Id: "home", FamilyId: "actual-family", Name: "Home", Notes: f.notes}, nil
}
func (f *notesHousingClient) UpdateHousingNotes(_ context.Context, req *pb.UpdateHousingNotesRequest, _ ...grpc.CallOption) (*pb.UpdateHousingNotesResponse, error) {
	f.request = req
	if f.saveErr != nil {
		return nil, f.saveErr
	}
	f.notes = req.Notes
	return &pb.UpdateHousingNotesResponse{Notes: f.notes}, nil
}

func TestHousingNotesEditor(t *testing.T) {
	templates, err := parseTemplates(filepath.Join("..", "..", "templates"))
	if err != nil {
		t.Fatal(err)
	}
	client := &notesHousingClient{notes: "  Viewing feedback\n</textarea><script>alert(1)</script>"}
	app := &App{clients: &core.Clients{Housing: client, Person: checklistPersonClient{}}}
	router := gin.New()
	router.SetHTMLTemplate(templates)
	router.Use(func(c *gin.Context) { c.Set(userContextKey, &CurrentUser{ID: "member", Role: "user"}) })
	router.GET("/housing/:id", app.housingDetail)
	router.POST("/housing/:id/notes", app.saveHousingNotes)
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}
	post := func(notes string) *httptest.ResponseRecorder {
		t.Helper()
		client.request = nil
		body := url.Values{"notes": {notes}, "family_id": {"wrong-family"}}
		req := httptest.NewRequest(http.MethodPost, "/housing/home/notes", strings.NewReader(body.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	w := get("/housing/home")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), html.EscapeString(client.notes)) || strings.Contains(w.Body.String(), "<script>alert(1)</script>") {
		t.Fatal("stored notes not safely displayed")
	}
	draft := strings.Repeat("a", 10001)
	w = post(draft)
	if client.request != nil || !strings.Contains(w.Body.String(), draft) || !strings.Contains(w.Body.String(), "Notes must be at most 10,000") {
		t.Fatal("oversized draft not preserved or sent to Core")
	}
	for _, code := range []codes.Code{codes.PermissionDenied, codes.Unavailable} {
		client.saveErr = status.Error(code, "Cannot save right now")
		w = post("Keep my draft <script>\nNext action")
		if !strings.Contains(w.Body.String(), "Keep my draft &lt;script&gt;\nNext action") || !strings.Contains(w.Body.String(), "Cannot save right now") {
			t.Fatal("failed save lost draft or error")
		}
	}
	client.saveErr = nil
	client.getErr = status.Error(codes.Unavailable, "Core is offline")
	client.saveErr = client.getErr
	w = post("Keep this draft during an outage")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Keep this draft during an outage") {
		t.Fatal("Core outage lost draft")
	}
	client.getErr, client.saveErr = nil, nil
	for _, notes := range []string{strings.Repeat("🏡", 10000), "  First line\r\nSecond line\r\n", ""} {
		w = post(notes)
		if w.Code != http.StatusFound || !strings.Contains(w.Header().Get("Location"), "#housing-notes") || client.request.GetHousingId() != "home" || client.notes != strings.ReplaceAll(notes, "\r\n", "\n") {
			t.Fatal("save or clear failed")
		}
	}
	w = get("/housing/home?flash=Housing+notes+saved")
	if !strings.Contains(w.Body.String(), "Housing notes saved") {
		t.Fatal("save confirmation missing")
	}
	if client.answer != nil {
		t.Fatal("notes save changed checklist")
	}
}
