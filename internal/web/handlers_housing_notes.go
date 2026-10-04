package web

import (
	"net/http"
	"strings"
	"unicode/utf8"

	pb "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
)

const housingNotesMaxLength = 10000

func (a *App) saveHousingNotes(c *gin.Context) {
	// Browsers submit CRLF for textarea line breaks; count/store the displayed LF.
	notes := strings.ReplaceAll(c.PostForm("notes"), "\r\n", "\n")
	message := ""
	if !utf8.ValidString(notes) || utf8.RuneCountInString(notes) > housingNotesMaxLength {
		message = "Notes must be at most 10,000 characters. Your changes have not been saved."
	} else {
		_, err := a.clients.Housing.UpdateHousingNotes(a.grpcContext(c), &pb.UpdateHousingNotesRequest{
			HousingId: c.Param("id"), Notes: notes,
		})
		if err != nil {
			message = "Unable to save notes: " + grpcMessage(err)
		}
	}
	if message != "" {
		c.Set("housing_notes_draft", notes)
		c.Set("housing_notes_error", message)
		a.housingDetail(c)
		return
	}
	c.Redirect(http.StatusFound, "/housing/"+urlQuerySafe(c.Param("id"))+"?flash=Housing+notes+saved#housing-notes")
}
