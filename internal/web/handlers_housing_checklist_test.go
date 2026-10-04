package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	pb "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
)

func checklistFormContext(values url.Values) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/housing/home/evaluation/location", strings.NewReader(values.Encode()))
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c.Params = gin.Params{{Key: "id", Value: "home"}, {Key: "criterion_id", Value: "location"}}
	return c
}
func TestHousingAnswerFormValidation(t *testing.T) {
	for _, test := range []struct {
		state, rating string
		valid         bool
	}{{"pending", "", true}, {"pending", "3", true}, {"complete", "5", true}, {"complete", "", false}, {"complete", "0", false}, {"pending", "6", false}, {"pending", "1.5", false}, {"pending", "NaN", false}, {"not_applicable", "4", true}, {"wrong", "", false}} {
		c := checklistFormContext(url.Values{"state": {test.state}, "rating": {test.rating}, "notes": {"Important notes"}})
		answer, err := housingAnswerFromForm(c)
		if (err == nil) != test.valid {
			t.Errorf("%+v: %v", test, err)
		}
		if answer.Notes != "Important notes" {
			t.Fatal("notes lost")
		}
		if test.state == "not_applicable" && answer.Rating != nil {
			t.Fatal("N/A rating not cleared")
		}
	}
}
func TestHousingCriterionFormValidation(t *testing.T) {
	for _, weight := range []string{"0", "-1", "1001", "NaN", "Inf", "abc"} {
		_, err := criterionFromForm(checklistFormContext(url.Values{"family_id": {"f"}, "name": {"Location"}, "display_order": {"1"}, "weight": {weight}}))
		if err == nil {
			t.Errorf("accepted weight %q", weight)
		}
	}
	criterion, err := criterionFromForm(checklistFormContext(url.Values{"family_id": {"f"}, "name": {" Location "}, "display_order": {"2"}, "weight": {"2.5"}}))
	if err != nil || criterion.Name != "Location" || criterion.DisplayOrder != 1 || criterion.GetWeight() != 2.5 {
		t.Fatalf("bad form mapping: %v %v", criterion, err)
	}
}

func TestHousingEvaluationTemplatesRender(t *testing.T) {
	templates, err := parseTemplates(filepath.Join("..", "..", "templates"))
	if err != nil {
		t.Fatal(err)
	}
	rating := int32(4)
	weight := 2.0
	score := 4.0
	criterion := &pb.HousingCriterion{Id: "location", FamilyId: "family", Name: "Location", Weight: &weight}
	evaluation := &pb.HousingEvaluation{Criteria: []*pb.HousingCriterion{criterion, {Id: "layout", Name: "Layout"}}, Answers: []*pb.HousingAnswer{{CriterionId: "location", HousingId: "home", State: "complete", Rating: &rating, Notes: "<script>alert('unsafe')</script> near family"}}, Summary: &pb.HousingEvaluationSummary{Score: &score, Total: 2, Completed: 1, Rated: 1, Weighted: true}}
	housing := &pb.HousingOption{Id: "home", FamilyId: "family", Name: "Sample home", Evaluation: evaluation.Summary}
	render := func(name string, data any) string {
		t.Helper()
		if view, ok := data.(ViewData); ok {
			view.User = &CurrentUser{ID: "viewer", Role: "user"}
			data = view
		}
		var b bytes.Buffer
		if err := templates.ExecuteTemplate(&b, name, data); err != nil {
			t.Fatalf("render %s: %v", name, err)
		}
		return b.String()
	}
	detail := housingDetailData{FamilyID: "family", Housing: housing, Evaluation: evaluation, EvaluationRows: evaluationRows(evaluation), Affordability: &pb.HousingAffordability{}}
	html := render("housing_detail", ViewData{Data: detail})
	for _, want := range []string{"4.00 / 5", "1 / 2 complete", "Unrated / pending", "Save Location", "/housing/home/evaluation/location", "&lt;script&gt;"} {
		if !strings.Contains(html, want) {
			t.Errorf("detail missing %q", want)
		}
	}
	if strings.Contains(html, "<script>alert('unsafe')</script>") {
		t.Fatal("notes not escaped")
	}
	render("housing_checklist", ViewData{Data: housingChecklistData{FamilyID: "family", Criteria: evaluation.Criteria, Draft: &pb.HousingCriterion{FamilyId: "family", DisplayOrder: 2}}})
	row := housingCompareRow{HousingID: "home", Name: "Sample home", Evaluation: evaluation, Affordability: &pb.HousingAffordability{}}
	html = render("housing_compare", ViewData{Data: housingCompareData{FamilyID: "family", Rows: []housingCompareRow{row}}})
	if strings.Count(html, "Criteria and notes") != 2 {
		t.Fatal("desktop and mobile comparison missing criteria")
	}
	render("housing", ViewData{Data: housingData{FamilyID: "family", GroupedSections: []housingGroupSection{{GroupName: "Homes", HousingOptions: []*pb.HousingOption{housing}}}}})
	render("dashboard", ViewData{Data: dashboardData{FamilyID: "family", Housing: []*pb.HousingOption{housing}, Dashboard: &pb.HouseholdDashboard{}, Income: &pb.HouseholdIncomeSummary{}}})
	detail.Evaluation = &pb.HousingEvaluation{Summary: &pb.HousingEvaluationSummary{}}
	detail.EvaluationRows = nil
	html = render("housing_detail", ViewData{Data: detail})
	if !strings.Contains(html, "No checklist yet") {
		t.Fatal("empty checklist missing")
	}
	detail.Evaluation = nil
	detail.EvaluationError = "Service unavailable"
	html = render("housing_detail", ViewData{Data: detail})
	if !strings.Contains(html, "Unable to load evaluation") {
		t.Fatal("error hidden")
	}
	if evaluationScore(nil) != "Evaluation unavailable" || evaluationScore(&pb.HousingEvaluationSummary{Total: 2}) != "Not rated" {
		t.Fatal("missing score treated as zero")
	}
}
