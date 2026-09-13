package web

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	pb "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
)

type housingChecklistData struct {
	FamilyID string
	Criteria []*pb.HousingCriterion
	Draft    *pb.HousingCriterion
}
type housingEvaluationRow struct {
	Criterion *pb.HousingCriterion
	Answer    *pb.HousingAnswer
}

func evaluationRows(e *pb.HousingEvaluation) []housingEvaluationRow {
	answers := map[string]*pb.HousingAnswer{}
	for _, answer := range e.GetAnswers() {
		answers[answer.GetCriterionId()] = answer
	}
	rows := []housingEvaluationRow{}
	for _, criterion := range e.GetCriteria() {
		answer := answers[criterion.GetId()]
		if answer == nil {
			answer = &pb.HousingAnswer{CriterionId: criterion.GetId(), State: "pending"}
		}
		rows = append(rows, housingEvaluationRow{criterion, answer})
	}
	return rows
}

func evaluationScore(s *pb.HousingEvaluationSummary) string {
	if s == nil {
		return "Evaluation unavailable"
	}
	if s.Total == 0 {
		return "No checklist yet"
	}
	if s.Score == nil {
		return "Not rated"
	}
	label := fmt.Sprintf("%.2f / 5", s.GetScore())
	if s.Completed < s.Total-s.NotApplicable {
		label += " (partial)"
	}
	return label
}
func criterionWeight(c *pb.HousingCriterion) string {
	if c.Weight == nil {
		return ""
	}
	return strconv.FormatFloat(c.GetWeight(), 'f', -1, 64)
}

func (a *App) housingChecklist(c *gin.Context) {
	a.renderHousingChecklist(c, strings.TrimSpace(c.Query("family_id")), nil, "")
}
func (a *App) renderHousingChecklist(c *gin.Context, familyID string, draft *pb.HousingCriterion, message string) {
	resp, err := a.clients.Housing.ListHousingCriteria(a.grpcContext(c), &pb.ListHousingCriteriaRequest{FamilyId: familyID})
	if err != nil {
		c.Redirect(http.StatusFound, "/housing?family_id="+urlQuerySafe(familyID)+"&error="+urlQuerySafe(grpcMessage(err)))
		return
	}
	if draft == nil {
		draft = &pb.HousingCriterion{FamilyId: familyID, DisplayOrder: int32(len(resp.GetCriteria()))}
	}
	if message != "" {
		setHousingFormError(c, message)
	}
	criteria := resp.GetCriteria()
	if draft.Id != "" {
		for i, criterion := range criteria {
			if criterion.Id == draft.Id {
				criteria[i] = draft
			}
		}
	}
	a.render(c, "housing_checklist", "Housing Checklist", housingChecklistData{FamilyID: familyID, Criteria: criteria, Draft: draft})
}
func setHousingFormError(c *gin.Context, message string) {
	query := c.Request.URL.Query()
	query.Set("error", message)
	c.Request.URL.RawQuery = query.Encode()
}

func criterionFromForm(c *gin.Context) (*pb.HousingCriterion, error) {
	criterion := &pb.HousingCriterion{Id: c.Param("criterion_id"), FamilyId: strings.TrimSpace(c.PostForm("family_id")), Name: strings.TrimSpace(c.PostForm("name")), Description: strings.TrimSpace(c.PostForm("description"))}
	order, err := strconv.ParseInt(c.PostForm("display_order"), 10, 32)
	if err != nil || order < 1 {
		return criterion, fmt.Errorf("Position must be a positive whole number")
	}
	criterion.DisplayOrder = int32(order - 1)
	if raw := strings.TrimSpace(c.PostForm("weight")); raw != "" {
		weight, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(weight) || math.IsInf(weight, 0) || weight <= 0 || weight > 1000 {
			return criterion, fmt.Errorf("Weight must be greater than 0 and at most 1000")
		}
		criterion.Weight = &weight
	}
	if criterion.Name == "" || utf8.RuneCountInString(criterion.Name) > 120 || utf8.RuneCountInString(criterion.Description) > 2000 {
		return criterion, fmt.Errorf("Enter a name of 1–120 characters and a description of at most 2000 characters")
	}
	return criterion, nil
}
func (a *App) saveHousingCriterion(c *gin.Context) {
	criterion, err := criterionFromForm(c)
	if err == nil {
		_, err = a.clients.Housing.SaveHousingCriterion(a.grpcContext(c), criterion)
	}
	if err != nil {
		a.renderHousingChecklist(c, criterion.FamilyId, criterion, grpcMessage(err))
		return
	}
	c.Redirect(http.StatusFound, "/housing/checklist?family_id="+urlQuerySafe(criterion.FamilyId)+"&flash=Checklist+saved")
}
func (a *App) deleteHousingCriterion(c *gin.Context) {
	familyID := strings.TrimSpace(c.PostForm("family_id"))
	_, err := a.clients.Housing.DeleteHousingCriterion(a.grpcContext(c), &pb.DeleteHousingCriterionRequest{FamilyId: familyID, CriterionId: c.Param("criterion_id")})
	if err != nil {
		a.renderHousingChecklist(c, familyID, nil, grpcMessage(err))
		return
	}
	c.Redirect(http.StatusFound, "/housing/checklist?family_id="+urlQuerySafe(familyID)+"&flash=Criterion+deleted")
}

func housingAnswerFromForm(c *gin.Context) (*pb.HousingAnswer, error) {
	answer := &pb.HousingAnswer{HousingId: c.Param("id"), CriterionId: c.Param("criterion_id"), State: c.PostForm("state"), Notes: c.PostForm("notes")}
	if raw := strings.TrimSpace(c.PostForm("rating")); raw != "" {
		rating, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || rating < 1 || rating > 5 {
			return answer, fmt.Errorf("Choose a rating from 1 to 5, or leave it unrated")
		}
		v := int32(rating)
		answer.Rating = &v
	}
	if answer.State != "pending" && answer.State != "complete" && answer.State != "not_applicable" {
		return answer, fmt.Errorf("Choose Pending, Complete, or Not applicable")
	}
	if answer.State == "complete" && answer.Rating == nil {
		return answer, fmt.Errorf("Add a rating before marking this criterion complete")
	}
	if answer.State == "not_applicable" {
		answer.Rating = nil
	}
	if utf8.RuneCountInString(answer.Notes) > 5000 {
		return answer, fmt.Errorf("Notes must be at most 5000 characters")
	}
	return answer, nil
}
func (a *App) saveHousingAnswer(c *gin.Context) {
	answer, err := housingAnswerFromForm(c)
	if err == nil {
		_, err = a.clients.Housing.SaveHousingAnswer(a.grpcContext(c), answer)
	}
	if err != nil {
		c.Set("housing_answer_draft", answer)
		setHousingFormError(c, grpcMessage(err))
		a.housingDetail(c)
		return
	}
	c.Redirect(http.StatusFound, "/housing/"+urlQuerySafe(answer.HousingId)+"?flash=Evaluation+saved#evaluation")
}
