package web

import (
	"net/http"
	"strings"

	ourneztv1 "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
)

type peopleData struct {
	FamilyID string
	People   []*ourneztv1.PersonProfile
}

type personFormData struct {
	FamilyID string
	Person   *ourneztv1.PersonProfile
	IsEdit   bool
}

func (a *App) people(c *gin.Context) {
	user := userFromContext(c)
	familyID := strings.TrimSpace(c.Query("family_id"))
	if familyID == "" {
		a.render(c, "people", "People", peopleData{})
		return
	}

	resp, err := a.clients.Person.ListPersonProfilesByFamily(a.grpcContext(c), &ourneztv1.ListPersonProfilesByFamilyRequest{
		ViewerUserId: user.ID,
		FamilyId:     familyID,
	})
	if err != nil {
		c.Redirect(http.StatusFound, "/dashboard?family_id="+familyID+"&error="+urlQuerySafe(grpcMessage(err)))
		return
	}
	a.render(c, "people", "People", peopleData{FamilyID: familyID, People: resp.GetPeople()})
}

func (a *App) newPerson(c *gin.Context) {
	a.render(c, "person_form", "New Person", personFormData{FamilyID: c.Query("family_id")})
}

func (a *App) createPerson(c *gin.Context) {
	person := personFromForm(c)
	person.Id = ""
	person.FamilyId = strings.TrimSpace(c.PostForm("family_id"))

	_, err := a.clients.Person.CreatePersonProfile(a.grpcContext(c), person)
	if err != nil {
		c.Redirect(http.StatusFound, "/people/new?family_id="+person.GetFamilyId()+"&error="+urlQuerySafe(grpcMessage(err)))
		return
	}
	c.Redirect(http.StatusFound, "/people?family_id="+person.GetFamilyId()+"&flash=Person+created")
}

func (a *App) editPerson(c *gin.Context) {
	user := userFromContext(c)
	familyID := c.Query("family_id")
	resp, err := a.clients.Person.GetPersonProfile(a.grpcContext(c), &ourneztv1.GetPersonProfileRequest{
		ViewerUserId: user.ID,
		PersonId:     c.Param("id"),
	})
	if err != nil {
		c.Redirect(http.StatusFound, "/people?family_id="+familyID+"&error="+urlQuerySafe(grpcMessage(err)))
		return
	}
	a.render(c, "person_form", "Edit Person", personFormData{FamilyID: familyID, Person: resp, IsEdit: true})
}

func (a *App) updatePerson(c *gin.Context) {
	person := personFromForm(c)
	person.Id = c.Param("id")
	person.FamilyId = strings.TrimSpace(c.PostForm("family_id"))

	_, err := a.clients.Person.UpdatePersonProfile(a.grpcContext(c), person)
	if err != nil {
		c.Redirect(http.StatusFound, "/people/"+person.GetId()+"/edit?family_id="+person.GetFamilyId()+"&error="+urlQuerySafe(grpcMessage(err)))
		return
	}
	c.Redirect(http.StatusFound, "/people?family_id="+person.GetFamilyId()+"&flash=Person+updated")
}

func (a *App) deletePerson(c *gin.Context) {
	user := userFromContext(c)
	familyID := strings.TrimSpace(c.PostForm("family_id"))
	_, err := a.clients.Person.DeletePersonProfile(a.grpcContext(c), &ourneztv1.DeletePersonProfileRequest{
		ActorUserId: user.ID,
		PersonId:    c.Param("id"),
	})
	if err != nil {
		c.Redirect(http.StatusFound, "/people?family_id="+familyID+"&error="+urlQuerySafe(grpcMessage(err)))
		return
	}
	c.Redirect(http.StatusFound, "/people?family_id="+familyID+"&flash=Person+deleted")
}

func personFromForm(c *gin.Context) *ourneztv1.PersonProfile {
	return &ourneztv1.PersonProfile{
		Name:                      strings.TrimSpace(c.PostForm("name")),
		Age:                       parseInt32(c.PostForm("age")),
		RelationshipLabel:         strings.TrimSpace(c.PostForm("relationship_label")),
		EmploymentStatus:          strings.TrimSpace(c.PostForm("employment_status")),
		GrossMonthlyIncomeCents:   parseInt64(c.PostForm("gross_monthly_income_cents")),
		ExpectedFutureIncomeCents: parseInt64(c.PostForm("expected_future_income_cents")),
		ExpectedIncomeStartDate:   strings.TrimSpace(c.PostForm("expected_income_start_date")),
		GraduationDate:            strings.TrimSpace(c.PostForm("graduation_date")),
		OrdDate:                   strings.TrimSpace(c.PostForm("ord_date")),
		CashSavingsCents:          parseInt64(c.PostForm("cash_savings_cents")),
		CpfOaCents:                parseInt64(c.PostForm("cpf_oa_cents")),
		CpfSaCents:                parseInt64(c.PostForm("cpf_sa_cents")),
		CpfMaCents:                parseInt64(c.PostForm("cpf_ma_cents")),
		MonthlyExpensesCents:      parseInt64(c.PostForm("monthly_expenses_cents")),
	}
}
