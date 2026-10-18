package web

import (
	"net/http"

	pb "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
)

const coreTimeoutMessage = "OurNeZt took too long to respond. Please try again."
const coreUnavailableMessage = "OurNeZt is temporarily unavailable. Please try again later."

type coreFailureData struct {
	Logout    bool
	Notes     *housingDetailData
	Answer    *pb.HousingAnswer
	Criterion *pb.HousingCriterion
}

func (a *App) renderCoreFailure(c *gin.Context, code codes.Code) bool {
	var statusCode int
	var title, message string
	switch code {
	case codes.DeadlineExceeded:
		statusCode, title, message = http.StatusGatewayTimeout, "Request timed out", coreTimeoutMessage
	case codes.Unavailable:
		statusCode, title, message = http.StatusServiceUnavailable, "Service temporarily unavailable", coreUnavailableMessage
	default:
		return false
	}
	drafts := coreFailureData{Logout: c.Request.Method == http.MethodPost && c.Request.URL.Path == "/logout"}
	if notes, ok := c.Get("housing_notes_draft"); ok {
		drafts.Notes = &housingDetailData{Housing: &pb.HousingOption{Id: c.Param("id")}, Notes: notes.(string), NotesError: message, NotesMaxLength: housingNotesMaxLength}
	}
	if answer, ok := c.Get("housing_answer_draft"); ok {
		drafts.Answer = answer.(*pb.HousingAnswer)
	}
	if criterion, ok := c.Get("housing_criterion_draft"); ok {
		drafts.Criterion = criterion.(*pb.HousingCriterion)
	}
	c.Header("Cache-Control", "no-store")
	c.Abort()
	c.HTML(statusCode, "core_unavailable", ViewData{
		Title: title, User: userFromContext(c), Error: message, MaintenanceNotice: a.maintenanceNotice(), Data: drafts,
	})
	return true
}
