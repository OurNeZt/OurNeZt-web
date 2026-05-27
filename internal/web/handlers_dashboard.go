package web

import (
	"net/http"

	ourneztv1 "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
)

type dashboardData struct {
	FamilyID  string
	Families  []*ourneztv1.Family
	Dashboard *ourneztv1.HouseholdDashboard
	People    []*ourneztv1.PersonProfile
	Housing   []*ourneztv1.HousingOption
	Income    *ourneztv1.HouseholdIncomeSummary
}

func (a *App) dashboard(c *gin.Context) {
	user := userFromContext(c)
	if user != nil && user.Role == "admin" {
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	fResp, err := a.clients.Family.ListUserFamilies(a.grpcContext(c), &ourneztv1.ListUserFamiliesRequest{UserId: user.ID})
	if err != nil {
		c.Redirect(http.StatusFound, "/families?error="+urlQuerySafe(grpcMessage(err)))
		return
	}
	families := fResp.GetFamilies()
	if len(families) == 0 {
		a.render(c, "dashboard", "Dashboard", dashboardData{Families: families})
		return
	}

	familyID := c.Query("family_id")
	if familyID == "" {
		familyID = families[0].GetId()
	}

	dResp, dashErr := a.clients.Dashboard.GetHouseholdDashboard(a.grpcContext(c), &ourneztv1.GetHouseholdDashboardRequest{
		ViewerUserId: user.ID,
		FamilyId:     familyID,
	})
	if dashErr != nil {
		c.Redirect(http.StatusFound, "/families?error="+urlQuerySafe(grpcMessage(dashErr)))
		return
	}

	peopleResp, _ := a.clients.Person.ListPersonProfilesByFamily(a.grpcContext(c), &ourneztv1.ListPersonProfilesByFamilyRequest{
		ViewerUserId: user.ID,
		FamilyId:     familyID,
	})
	housingResp, _ := a.clients.Housing.ListHousingOptions(a.grpcContext(c), &ourneztv1.ListHousingOptionsRequest{
		ViewerUserId: user.ID,
		FamilyId:     familyID,
	})
	incomeResp, _ := a.clients.Income.CalculateHouseholdIncomeSummary(a.grpcContext(c), &ourneztv1.CalculateHouseholdIncomeSummaryRequest{
		People: peopleResp.GetPeople(),
	})

	a.render(c, "dashboard", "Dashboard", dashboardData{
		FamilyID:  familyID,
		Families:  families,
		Dashboard: dResp,
		People:    peopleResp.GetPeople(),
		Housing:   housingResp.GetHousingOptions(),
		Income:    incomeResp,
	})
}
