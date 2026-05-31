package web

import (
	"net/http"
	"strings"

	ourneztv1 "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
)

type peopleData struct {
	FamilyID string
	Families []*ourneztv1.Family
	Rows     []peopleRow
}

type peopleRow struct {
	Person *ourneztv1.PersonProfile
	IsSelf bool
}

type personFormData struct {
	FamilyID       string
	Person         *ourneztv1.PersonProfile
	IsEdit         bool
	ManagedByOwner bool
	ReturnTo       string
	FormAction     string
}

func (a *App) people(c *gin.Context) {
	user := userFromContext(c)
	fResp, fErr := a.clients.Family.ListUserFamilies(a.grpcContext(c), &ourneztv1.ListUserFamiliesRequest{
		UserId: user.ID,
	})
	if fErr != nil {
		c.Redirect(http.StatusFound, "/dashboard?error="+urlQuerySafe(grpcMessage(fErr)))
		return
	}
	families := fResp.GetFamilies()

	familyID := strings.TrimSpace(c.Query("family_id"))
	if familyID == "" {
		if len(families) == 0 {
			a.render(c, "people", "Household Profiles", peopleData{Families: families})
			return
		}
		c.Redirect(http.StatusFound, "/people?family_id="+families[0].GetId())
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

	rows := make([]peopleRow, 0, len(resp.GetPeople()))
	for _, person := range resp.GetPeople() {
		rows = append(rows, peopleRow{
			Person: person,
			IsSelf: matchesCurrentUserProfile(user, person),
		})
	}

	a.render(c, "people", "Household Profiles", peopleData{
		FamilyID: familyID,
		Families: families,
		Rows:     rows,
	})
}

func (a *App) newPerson(c *gin.Context) {
	a.render(c, "person_form", "Add Occupant", personFormData{
		FamilyID:       c.Query("family_id"),
		Person:         &ourneztv1.PersonProfile{},
		ManagedByOwner: true,
		ReturnTo:       "/people?family_id=" + strings.TrimSpace(c.Query("family_id")),
		FormAction:     "/people",
	})
}

func (a *App) createPerson(c *gin.Context) {
	person := personFromForm(c)
	person.Id = ""
	person.FamilyId = strings.TrimSpace(c.PostForm("family_id"))
	if validationErr := validatePersonProfileInput(person); validationErr != "" {
		c.Redirect(http.StatusFound, "/people/new?family_id="+person.GetFamilyId()+"&error="+urlQuerySafe(validationErr))
		return
	}

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
	if matchesCurrentUserProfile(user, resp) {
		c.Redirect(http.StatusFound, "/profile?error=Edit+your+own+profile+from+My+Profile")
		return
	}
	a.render(c, "person_form", "Edit Occupant", personFormData{
		FamilyID:       familyID,
		Person:         resp,
		IsEdit:         true,
		ManagedByOwner: true,
		ReturnTo:       "/people?family_id=" + familyID,
		FormAction:     "/people/" + resp.GetId(),
	})
}

func (a *App) updatePerson(c *gin.Context) {
	user := userFromContext(c)
	person := personFromForm(c)
	person.Id = c.Param("id")
	person.FamilyId = strings.TrimSpace(c.PostForm("family_id"))
	if validationErr := validatePersonProfileInput(person); validationErr != "" {
		c.Redirect(http.StatusFound, "/people/"+person.GetId()+"/edit?family_id="+person.GetFamilyId()+"&error="+urlQuerySafe(validationErr))
		return
	}

	current, currentErr := a.clients.Person.GetPersonProfile(a.grpcContext(c), &ourneztv1.GetPersonProfileRequest{
		ViewerUserId: user.ID,
		PersonId:     person.GetId(),
	})
	if currentErr == nil && matchesCurrentUserProfile(user, current) {
		c.Redirect(http.StatusFound, "/profile?error=Edit+your+own+profile+from+My+Profile")
		return
	}

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
	current, currentErr := a.clients.Person.GetPersonProfile(a.grpcContext(c), &ourneztv1.GetPersonProfileRequest{
		ViewerUserId: user.ID,
		PersonId:     c.Param("id"),
	})
	if currentErr == nil && matchesCurrentUserProfile(user, current) {
		c.Redirect(http.StatusFound, "/profile?error=Delete+your+own+profile+from+My+Profile+if+needed")
		return
	}
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
		RelationshipLabel:         normalizeLookup(c.PostForm("relationship_label")),
		EmploymentStatus:          normalizeLookup(c.PostForm("employment_status")),
		GrossMonthlyIncomeCents:   parseMoneyCents(c.PostForm("gross_monthly_income")),
		ExpectedFutureIncomeCents: parseMoneyCents(c.PostForm("expected_future_income")),
		ExpectedIncomeStartDate:   strings.TrimSpace(c.PostForm("expected_income_start_date")),
		GraduationDate:            strings.TrimSpace(c.PostForm("graduation_date")),
		OrdDate:                   strings.TrimSpace(c.PostForm("ord_date")),
		CashSavingsCents:          parseMoneyCents(c.PostForm("cash_savings")),
		CpfOaCents:                parseMoneyCents(c.PostForm("cpf_oa")),
		CpfSaCents:                parseMoneyCents(c.PostForm("cpf_sa")),
		CpfMaCents:                parseMoneyCents(c.PostForm("cpf_ma")),
		MonthlyExpensesCents:      parseMoneyCents(c.PostForm("monthly_expenses")),
	}
}

func validatePersonProfileInput(person *ourneztv1.PersonProfile) string {
	if person == nil {
		return "invalid profile payload"
	}

	if strings.TrimSpace(person.GetName()) == "" {
		return "name is required"
	}

	switch person.GetRelationshipLabel() {
	case "spouse", "fiance", "fiancee", "occupant":
	case "self", "me":
		person.RelationshipLabel = "occupant"
	default:
		return "relationship is required"
	}

	switch person.GetEmploymentStatus() {
	case "full_time_employee":
		person.ExpectedFutureIncomeCents = 0
		person.ExpectedIncomeStartDate = ""
		person.GraduationDate = ""
		person.OrdDate = ""
	case "student":
		person.OrdDate = ""
		if !isISODate(person.GetGraduationDate()) {
			return "graduation date is required for student (YYYY-MM-DD)"
		}
	case "full_time_nsf":
		person.GraduationDate = ""
		if !isISODate(person.GetOrdDate()) {
			return "ORD date is required for NSF (YYYY-MM-DD)"
		}
	default:
		return "employment status is required"
	}

	if strings.TrimSpace(person.GetExpectedIncomeStartDate()) != "" && !isISODate(person.GetExpectedIncomeStartDate()) {
		return "expected income start date must be YYYY-MM-DD"
	}

	return ""
}
