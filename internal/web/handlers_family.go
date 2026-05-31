package web

import (
	"net/http"
	"strings"

	ourneztv1 "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
)

type familiesData struct {
	Families []*ourneztv1.Family
}

type familyDetailData struct {
	Family   *ourneztv1.Family
	Members  []*ourneztv1.FamilyMember
	JoinCode string
}

func (a *App) families(c *gin.Context) {
	user := userFromContext(c)
	resp, err := a.clients.Family.ListUserFamilies(a.grpcContext(c), &ourneztv1.ListUserFamiliesRequest{UserId: user.ID})
	if err != nil {
		a.render(c, "families", "Families", familiesData{})
		return
	}
	data := familiesData{Families: resp.GetFamilies()}
	a.render(c, "families", "Families", data)
}

func (a *App) newFamily(c *gin.Context) {
	c.Redirect(http.StatusFound, "/families")
}

func (a *App) createFamily(c *gin.Context) {
	user := userFromContext(c)
	familyType := normalizeCreateFamilyType(strings.TrimSpace(c.PostForm("family_type")))
	if familyType == "" {
		c.Redirect(http.StatusFound, "/families?error="+urlQuerySafe("family type is required"))
		return
	}
	_, err := a.clients.Family.CreateFamily(a.grpcContext(c), &ourneztv1.CreateFamilyRequest{
		OwnerUserId: user.ID,
		Name:        strings.TrimSpace(c.PostForm("name")),
		FamilyType:  familyType,
	})
	if err != nil {
		c.Redirect(http.StatusFound, "/families?error="+urlQuerySafe(grpcMessage(err)))
		return
	}
	c.Redirect(http.StatusFound, "/families?flash=Family+created")
}

func normalizeCreateFamilyType(raw string) string {
	switch normalizeLookup(raw) {
	case "single":
		return "single"
	case "couple":
		return "couple"
	case "family":
		return "family"
	default:
		return ""
	}
}

func (a *App) familyDetail(c *gin.Context) {
	user := userFromContext(c)
	familyID := c.Param("id")
	family, err := a.clients.Family.GetFamily(a.grpcContext(c), &ourneztv1.GetFamilyRequest{
		ViewerUserId: user.ID,
		FamilyId:     familyID,
	})
	if err != nil {
		c.Redirect(http.StatusFound, "/families?error="+urlQuerySafe(grpcMessage(err)))
		return
	}

	membersResp, membersErr := a.clients.Family.ListFamilyMembers(a.grpcContext(c), &ourneztv1.ListFamilyMembersRequest{
		ActorUserId: user.ID,
		FamilyId:    familyID,
	})
	data := familyDetailData{Family: family}
	if membersErr == nil {
		data.Members = membersResp.GetMembers()
	}
	a.render(c, "family_detail", "Family", data)
}

func (a *App) joinFamily(c *gin.Context) {
	user := userFromContext(c)
	_, err := a.clients.Family.JoinFamilyByCode(a.grpcContext(c), &ourneztv1.JoinFamilyByCodeRequest{
		UserId: user.ID,
		Code:   strings.TrimSpace(c.PostForm("code")),
	})
	if err != nil {
		c.Redirect(http.StatusFound, "/families?error="+urlQuerySafe(grpcMessage(err)))
		return
	}
	c.Redirect(http.StatusFound, "/families?flash=Joined+family")
}

func (a *App) regenerateFamilyCode(c *gin.Context) {
	user := userFromContext(c)
	resp, err := a.clients.Family.GenerateFamilyJoinCode(a.grpcContext(c), &ourneztv1.GenerateFamilyJoinCodeRequest{
		ActorUserId: user.ID,
		FamilyId:    c.Param("id"),
	})
	if err != nil {
		c.Redirect(http.StatusFound, "/families/"+c.Param("id")+"?error="+urlQuerySafe(grpcMessage(err)))
		return
	}

	familyID := c.Param("id")
	family, _ := a.clients.Family.GetFamily(a.grpcContext(c), &ourneztv1.GetFamilyRequest{ViewerUserId: user.ID, FamilyId: familyID})
	membersResp, _ := a.clients.Family.ListFamilyMembers(a.grpcContext(c), &ourneztv1.ListFamilyMembersRequest{ActorUserId: user.ID, FamilyId: familyID})
	a.render(c, "family_detail", "Family", familyDetailData{Family: family, Members: membersResp.GetMembers(), JoinCode: resp.GetCode()})
}
