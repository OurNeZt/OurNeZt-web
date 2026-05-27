package web

import (
	"net/http"
	"strings"

	ourneztv1 "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
)

type housingData struct {
	FamilyID string
	Housing  []*ourneztv1.HousingOption
}

type housingFormData struct {
	FamilyID string
	Housing  *ourneztv1.HousingOption
	IsEdit   bool
}

type housingDetailData struct {
	FamilyID      string
	Housing       *ourneztv1.HousingOption
	Affordability *ourneztv1.HousingAffordability
}

type housingCompareData struct {
	FamilyID string
	Rows     []*ourneztv1.HousingAffordability
}

func (a *App) housing(c *gin.Context) {
	user := userFromContext(c)
	familyID := strings.TrimSpace(c.Query("family_id"))
	if familyID == "" {
		a.render(c, "housing", "Housing", housingData{})
		return
	}

	resp, err := a.clients.Housing.ListHousingOptions(a.grpcContext(c), &ourneztv1.ListHousingOptionsRequest{
		ViewerUserId: user.ID,
		FamilyId:     familyID,
	})
	if err != nil {
		c.Redirect(http.StatusFound, "/dashboard?family_id="+familyID+"&error="+urlQuerySafe(grpcMessage(err)))
		return
	}
	a.render(c, "housing", "Housing", housingData{FamilyID: familyID, Housing: resp.GetHousingOptions()})
}

func (a *App) newHousing(c *gin.Context) {
	a.render(c, "housing_form", "New Housing", housingFormData{FamilyID: c.Query("family_id")})
}

func (a *App) createHousing(c *gin.Context) {
	option := housingFromForm(c)
	option.Id = ""
	option.FamilyId = strings.TrimSpace(c.PostForm("family_id"))

	_, err := a.clients.Housing.CreateHousingOption(a.grpcContext(c), option)
	if err != nil {
		c.Redirect(http.StatusFound, "/housing/new?family_id="+option.GetFamilyId()+"&error="+urlQuerySafe(grpcMessage(err)))
		return
	}
	c.Redirect(http.StatusFound, "/housing?family_id="+option.GetFamilyId()+"&flash=Housing+option+created")
}

func (a *App) housingDetail(c *gin.Context) {
	user := userFromContext(c)
	familyID := c.Query("family_id")
	option, err := a.clients.Housing.GetHousingOption(a.grpcContext(c), &ourneztv1.GetHousingOptionRequest{
		ViewerUserId: user.ID,
		HousingId:    c.Param("id"),
	})
	if err != nil {
		c.Redirect(http.StatusFound, "/housing?family_id="+familyID+"&error="+urlQuerySafe(grpcMessage(err)))
		return
	}

	aff := &ourneztv1.HousingAffordability{}
	if familyID != "" {
		peopleResp, peopleErr := a.clients.Person.ListPersonProfilesByFamily(a.grpcContext(c), &ourneztv1.ListPersonProfilesByFamilyRequest{ViewerUserId: user.ID, FamilyId: familyID})
		if peopleErr == nil {
			summary, sumErr := a.clients.Income.CalculateHouseholdIncomeSummary(a.grpcContext(c), &ourneztv1.CalculateHouseholdIncomeSummaryRequest{People: peopleResp.GetPeople()})
			if sumErr == nil {
				affResp, affErr := a.clients.Housing.CalculateHousingAffordability(a.grpcContext(c), &ourneztv1.CalculateHousingAffordabilityRequest{
					HousingOption:        option,
					CashSavingsCents:     totalCash(peopleResp.GetPeople()),
					CpfOaCents:           summary.GetCurrentCpfOaCents(),
					TakeHomeCents:        summary.GetTakeHomeIncomeCents(),
					MonthlyExpensesCents: summary.GetMonthlyExpensesCents(),
				})
				if affErr == nil {
					aff = affResp
				}
			}
		}
	}

	a.render(c, "housing_detail", "Housing Detail", housingDetailData{FamilyID: familyID, Housing: option, Affordability: aff})
}

func (a *App) editHousing(c *gin.Context) {
	user := userFromContext(c)
	familyID := c.Query("family_id")
	resp, err := a.clients.Housing.GetHousingOption(a.grpcContext(c), &ourneztv1.GetHousingOptionRequest{
		ViewerUserId: user.ID,
		HousingId:    c.Param("id"),
	})
	if err != nil {
		c.Redirect(http.StatusFound, "/housing?family_id="+familyID+"&error="+urlQuerySafe(grpcMessage(err)))
		return
	}
	a.render(c, "housing_form", "Edit Housing", housingFormData{FamilyID: familyID, Housing: resp, IsEdit: true})
}

func (a *App) updateHousing(c *gin.Context) {
	option := housingFromForm(c)
	option.Id = c.Param("id")
	option.FamilyId = strings.TrimSpace(c.PostForm("family_id"))

	_, err := a.clients.Housing.UpdateHousingOption(a.grpcContext(c), option)
	if err != nil {
		c.Redirect(http.StatusFound, "/housing/"+option.GetId()+"/edit?family_id="+option.GetFamilyId()+"&error="+urlQuerySafe(grpcMessage(err)))
		return
	}
	c.Redirect(http.StatusFound, "/housing?family_id="+option.GetFamilyId()+"&flash=Housing+option+updated")
}

func (a *App) deleteHousing(c *gin.Context) {
	user := userFromContext(c)
	familyID := strings.TrimSpace(c.PostForm("family_id"))
	_, err := a.clients.Housing.DeleteHousingOption(a.grpcContext(c), &ourneztv1.DeleteHousingOptionRequest{
		ActorUserId: user.ID,
		HousingId:   c.Param("id"),
	})
	if err != nil {
		c.Redirect(http.StatusFound, "/housing?family_id="+familyID+"&error="+urlQuerySafe(grpcMessage(err)))
		return
	}
	c.Redirect(http.StatusFound, "/housing?family_id="+familyID+"&flash=Housing+option+deleted")
}

func (a *App) compareHousing(c *gin.Context) {
	user := userFromContext(c)
	familyID := strings.TrimSpace(c.Query("family_id"))
	if familyID == "" {
		a.render(c, "housing_compare", "Housing Compare", housingCompareData{})
		return
	}

	hResp, err := a.clients.Housing.ListHousingOptions(a.grpcContext(c), &ourneztv1.ListHousingOptionsRequest{
		ViewerUserId: user.ID,
		FamilyId:     familyID,
	})
	if err != nil {
		c.Redirect(http.StatusFound, "/housing?family_id="+familyID+"&error="+urlQuerySafe(grpcMessage(err)))
		return
	}
	pResp, _ := a.clients.Person.ListPersonProfilesByFamily(a.grpcContext(c), &ourneztv1.ListPersonProfilesByFamilyRequest{ViewerUserId: user.ID, FamilyId: familyID})
	summary, _ := a.clients.Income.CalculateHouseholdIncomeSummary(a.grpcContext(c), &ourneztv1.CalculateHouseholdIncomeSummaryRequest{People: pResp.GetPeople()})

	rows := make([]*ourneztv1.HousingAffordability, 0, len(hResp.GetHousingOptions()))
	for _, option := range hResp.GetHousingOptions() {
		row, rowErr := a.clients.Housing.CalculateHousingAffordability(a.grpcContext(c), &ourneztv1.CalculateHousingAffordabilityRequest{
			HousingOption:        option,
			CashSavingsCents:     totalCash(pResp.GetPeople()),
			CpfOaCents:           summary.GetCurrentCpfOaCents(),
			TakeHomeCents:        summary.GetTakeHomeIncomeCents(),
			MonthlyExpensesCents: summary.GetMonthlyExpensesCents(),
		})
		if rowErr == nil {
			rows = append(rows, row)
		}
	}

	a.render(c, "housing_compare", "Housing Compare", housingCompareData{FamilyID: familyID, Rows: rows})
}

func housingFromForm(c *gin.Context) *ourneztv1.HousingOption {
	return &ourneztv1.HousingOption{
		Name:                      strings.TrimSpace(c.PostForm("name")),
		HousingType:               strings.TrimSpace(c.PostForm("housing_type")),
		Location:                  strings.TrimSpace(c.PostForm("location")),
		UnitType:                  strings.TrimSpace(c.PostForm("unit_type")),
		PurchasePriceCents:        parseInt64(c.PostForm("purchase_price_cents")),
		GrantAmountCents:          parseInt64(c.PostForm("grant_amount_cents")),
		LoanType:                  strings.TrimSpace(c.PostForm("loan_type")),
		LoanAmountCents:           parseInt64(c.PostForm("loan_amount_cents")),
		InterestRateBps:           parseInt64(c.PostForm("interest_rate_bps")),
		LoanTenureMonths:          parseInt32(c.PostForm("loan_tenure_months")),
		DownpaymentPercentBps:     parseInt64(c.PostForm("downpayment_percent_bps")),
		RenovationBudgetCents:     parseInt64(c.PostForm("renovation_budget_cents")),
		FurnitureBudgetCents:      parseInt64(c.PostForm("furniture_budget_cents")),
		LegalFeesCents:            parseInt64(c.PostForm("legal_fees_cents")),
		BuyerStampDutyCents:       parseInt64(c.PostForm("buyer_stamp_duty_cents")),
		MonthlyMaintenanceCents:   parseInt64(c.PostForm("monthly_maintenance_cents")),
		ExpectedKeyCollectionDate: strings.TrimSpace(c.PostForm("expected_key_collection_date")),
	}
}

func totalCash(people []*ourneztv1.PersonProfile) int64 {
	var total int64
	for _, person := range people {
		total += person.GetCashSavingsCents()
	}
	return total
}
