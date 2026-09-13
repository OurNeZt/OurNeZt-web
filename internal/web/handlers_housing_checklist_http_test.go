package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OurNeZt/ournezt-web/internal/config"
	"github.com/OurNeZt/ournezt-web/internal/core"
	pb "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type checklistHousingClient struct {
	pb.HousingServiceClient
	answer  *pb.HousingAnswer
	saveErr error
}

func (f *checklistHousingClient) GetHousingOption(context.Context, *pb.GetHousingOptionRequest, ...grpc.CallOption) (*pb.HousingOption, error) {
	return &pb.HousingOption{Id: "home", FamilyId: "actual-family", Name: "Home"}, nil
}
func (f *checklistHousingClient) GetHousingEvaluation(context.Context, *pb.GetHousingOptionRequest, ...grpc.CallOption) (*pb.HousingEvaluation, error) {
	return &pb.HousingEvaluation{Criteria: []*pb.HousingCriterion{{Id: "location", Name: "Location"}}, Summary: &pb.HousingEvaluationSummary{Total: 1}}, nil
}
func (f *checklistHousingClient) SaveHousingAnswer(_ context.Context, a *pb.HousingAnswer, _ ...grpc.CallOption) (*pb.HousingAnswer, error) {
	f.answer = a
	return a, f.saveErr
}

type checklistPersonClient struct{ pb.PersonServiceClient }

func (checklistPersonClient) ListPersonProfilesByFamily(context.Context, *pb.ListPersonProfilesByFamilyRequest, ...grpc.CallOption) (*pb.ListPersonProfilesByFamilyResponse, error) {
	return nil, status.Error(codes.Unavailable, "unavailable")
}

func TestSaveHousingAnswerPreservesNotesOnFailure(t *testing.T) {
	templates, err := parseTemplates(filepath.Join("..", "..", "templates"))
	if err != nil {
		t.Fatal(err)
	}
	client := &checklistHousingClient{}
	app := &App{clients: &core.Clients{Housing: client, Person: checklistPersonClient{}}}
	router := gin.New()
	router.SetHTMLTemplate(templates)
	router.Use(func(c *gin.Context) { c.Set(userContextKey, &CurrentUser{ID: "viewer", Role: "user"}) })
	router.POST("/housing/:id/evaluation/:criterion_id", app.saveHousingAnswer)
	post := func(state, rating string) *httptest.ResponseRecorder {
		t.Helper()
		body := url.Values{"state": {state}, "rating": {rating}, "notes": {"Keep this important note"}}
		req := httptest.NewRequest(http.MethodPost, "/housing/home/evaluation/location?family_id=wrong-family", strings.NewReader(body.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	w := post("complete", "")
	if w.Code != http.StatusOK || client.answer != nil || !strings.Contains(w.Body.String(), "Keep this important note") || !strings.Contains(w.Body.String(), "Add a rating before") || !strings.Contains(w.Body.String(), "family_id=actual-family") {
		t.Fatalf("validation lost draft or canonical family: %d %s", w.Code, w.Body.String())
	}
	client.saveErr = status.Error(codes.PermissionDenied, "You cannot edit this family")
	w = post("complete", "4")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Keep this important note") || !strings.Contains(w.Body.String(), "You cannot edit this family") {
		t.Fatalf("RPC error lost draft: %d %s", w.Code, w.Body.String())
	}
	client.saveErr = nil
	w = post("complete", "4")
	if w.Code != http.StatusFound || !strings.Contains(w.Header().Get("Location"), "#evaluation") || client.answer.GetRating() != 4 {
		t.Fatalf("save failed: %d %v", w.Code, client.answer)
	}
}

func TestChecklistRoutesRegister(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	if _, err := NewRouter(config.Config{}, &core.Clients{}); err != nil {
		t.Fatal(err)
	}
}
