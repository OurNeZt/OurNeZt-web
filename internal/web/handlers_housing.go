package web

import (
	"math"
	"net/http"
	"strings"

	ourneztv1 "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
)

type housingData struct {
	FamilyID string
	Families []*ourneztv1.Family
	Housing  []*ourneztv1.HousingOption
}

type housingFormData struct {
	FamilyID       string
	Housing        *ourneztv1.HousingOption
	IsEdit         bool
	AssessmentMode string
}

type housingDetailData struct {
	FamilyID      string
	Housing       *ourneztv1.HousingOption
	Affordability *ourneztv1.HousingAffordability
}

type housingCompareData struct {
	FamilyID string
	Families []*ourneztv1.Family
	Rows     []housingCompareRow
}

type housingCompareRow struct {
	Name          string
	Affordability *ourneztv1.HousingAffordability
}

func (a *App) housing(c *gin.Context) {
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
			a.render(c, "housing", "Housing", housingData{Families: families})
			return
		}
		c.Redirect(http.StatusFound, "/housing?family_id="+families[0].GetId())
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
	a.render(c, "housing", "Housing", housingData{
		FamilyID: familyID,
		Families: families,
		Housing:  resp.GetHousingOptions(),
	})
}

func (a *App) newHousing(c *gin.Context) {
	a.render(c, "housing_form", "New Housing", housingFormData{
		FamilyID:       c.Query("family_id"),
		Housing:        &ourneztv1.HousingOption{},
		AssessmentMode: "standard",
	})
}

func (a *App) createHousing(c *gin.Context) {
	assessmentMode := normalizeHousingAssessmentMode(c.PostForm("assessment_mode"))
	option := housingFromForm(c)
	option.Id = ""
	option.FamilyId = strings.TrimSpace(c.PostForm("family_id"))
	if validationErr := validateHousingOptionInput(option, assessmentMode); validationErr != "" {
		c.Redirect(http.StatusFound, "/housing/new?family_id="+option.GetFamilyId()+"&error="+urlQuerySafe(validationErr))
		return
	}
	normalizeHousingOption(option, assessmentMode)

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
	a.render(c, "housing_form", "Edit Housing", housingFormData{
		FamilyID:       familyID,
		Housing:        resp,
		IsEdit:         true,
		AssessmentMode: inferHousingAssessmentMode(resp),
	})
}

func (a *App) updateHousing(c *gin.Context) {
	assessmentMode := normalizeHousingAssessmentMode(c.PostForm("assessment_mode"))
	option := housingFromForm(c)
	option.Id = c.Param("id")
	option.FamilyId = strings.TrimSpace(c.PostForm("family_id"))
	if validationErr := validateHousingOptionInput(option, assessmentMode); validationErr != "" {
		c.Redirect(http.StatusFound, "/housing/"+option.GetId()+"/edit?family_id="+option.GetFamilyId()+"&error="+urlQuerySafe(validationErr))
		return
	}
	normalizeHousingOption(option, assessmentMode)

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
			a.render(c, "housing_compare", "Housing Compare", housingCompareData{Families: families})
			return
		}
		c.Redirect(http.StatusFound, "/housing/compare?family_id="+families[0].GetId())
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

	rows := make([]housingCompareRow, 0, len(hResp.GetHousingOptions()))
	for _, option := range hResp.GetHousingOptions() {
		row, rowErr := a.clients.Housing.CalculateHousingAffordability(a.grpcContext(c), &ourneztv1.CalculateHousingAffordabilityRequest{
			HousingOption:        option,
			CashSavingsCents:     totalCash(pResp.GetPeople()),
			CpfOaCents:           summary.GetCurrentCpfOaCents(),
			TakeHomeCents:        summary.GetTakeHomeIncomeCents(),
			MonthlyExpensesCents: summary.GetMonthlyExpensesCents(),
		})
		if rowErr == nil {
			rows = append(rows, housingCompareRow{
				Name:          option.GetName(),
				Affordability: row,
			})
		}
	}

	a.render(c, "housing_compare", "Housing Compare", housingCompareData{
		FamilyID: familyID,
		Families: families,
		Rows:     rows,
	})
}

func housingFromForm(c *gin.Context) *ourneztv1.HousingOption {
	return &ourneztv1.HousingOption{
		Name:                      strings.TrimSpace(c.PostForm("name")),
		HousingType:               normalizeLookup(c.PostForm("housing_type")),
		Location:                  strings.TrimSpace(c.PostForm("location")),
		UnitType:                  strings.TrimSpace(c.PostForm("unit_type")),
		PurchasePriceCents:        parseMoneyCents(c.PostForm("purchase_price")),
		GrantAmountCents:          parseMoneyCents(c.PostForm("grant_amount")),
		LoanType:                  normalizeLookup(c.PostForm("loan_type")),
		LoanAmountCents:           parseMoneyCents(c.PostForm("loan_amount")),
		InterestRateBps:           parsePercentBps(c.PostForm("interest_rate_percent")),
		LoanTenureMonths:          parseInt32(c.PostForm("loan_tenure_months")),
		DownpaymentPercentBps:     0,
		RenovationBudgetCents:     parseMoneyCents(c.PostForm("renovation_budget")),
		FurnitureBudgetCents:      parseMoneyCents(c.PostForm("furniture_budget")),
		LegalFeesCents:            parseMoneyCents(c.PostForm("legal_fees")),
		BuyerStampDutyCents:       0,
		MonthlyMaintenanceCents:   parseMoneyCents(c.PostForm("monthly_maintenance")),
		ExpectedKeyCollectionDate: strings.TrimSpace(c.PostForm("expected_key_collection_date")),
	}
}

func validateHousingOptionInput(option *ourneztv1.HousingOption, assessmentMode string) string {
	if option == nil {
		return "invalid housing payload"
	}

	if strings.TrimSpace(option.GetName()) == "" {
		return "housing name is required"
	}
	switch option.GetHousingType() {
	case "bto", "resale_hdb", "executive_condo", "private_condo", "landed", "other":
	default:
		return "housing type is required"
	}
	if strings.TrimSpace(option.GetUnitType()) == "" {
		return "unit type is required"
	}
	if option.GetPurchasePriceCents() <= 0 {
		return "purchase price is required"
	}
	netPurchase := maxInt64(option.GetPurchasePriceCents()-option.GetGrantAmountCents(), 0)
	if assessmentMode == "deferred" {
		// DIA mode intentionally omits grant + loan details.
	} else {
		switch option.GetLoanType() {
		case "hdb", "bank", "cash":
		default:
			return "loan type is required"
		}
	}
	if option.GetLoanType() == "cash" {
		// No extra checks required for cash purchase.
	} else if assessmentMode == "deferred" {
		// No loan fields required from user in DIA mode.
	} else {
		if option.GetLoanAmountCents() <= 0 {
			return "loan amount is required for standard assessment"
		}
		if netPurchase > 0 && option.GetLoanAmountCents() > netPurchase {
			return "loan amount cannot exceed net purchase price"
		}
		if option.GetLoanTenureMonths() <= 0 {
			return "loan tenure is required"
		}
	}
	if option.GetInterestRateBps() < 0 {
		return "interest rate cannot be negative"
	}
	if option.GetLoanTenureMonths() < 0 {
		return "loan tenure cannot be negative"
	}
	if option.GetExpectedKeyCollectionDate() != "" && !isISODate(option.GetExpectedKeyCollectionDate()) {
		return "expected key collection date must be YYYY-MM-DD"
	}
	return ""
}

func normalizeHousingAssessmentMode(raw string) string {
	switch normalizeLookup(raw) {
	case "deferred":
		return "deferred"
	default:
		return "standard"
	}
}

func inferHousingAssessmentMode(option *ourneztv1.HousingOption) string {
	if option == nil {
		return "standard"
	}
	if option.GetLoanType() != "cash" && option.GetLoanAmountCents() == 0 && option.GetDownpaymentPercentBps() == 2500 {
		return "deferred"
	}
	return "standard"
}

func normalizeHousingOption(option *ourneztv1.HousingOption, assessmentMode string) {
	if option == nil {
		return
	}

	// Temporarily managed elsewhere as requested.
	option.RenovationBudgetCents = 0
	option.FurnitureBudgetCents = 0

	option.BuyerStampDutyCents = calculateResidentialBSDCents(option.GetPurchasePriceCents())

	if option.GetLoanType() == "hdb" {
		// HDB concessionary interest is treated as fixed at 2.60%.
		option.InterestRateBps = 260
	}

	if option.GetLoanType() == "cash" {
		option.LoanAmountCents = 0
		option.InterestRateBps = 0
		option.LoanTenureMonths = 0
		option.DownpaymentPercentBps = 10000
		return
	}

	if assessmentMode == "deferred" {
		// Deferred Income Assessment assumption: 75% max loan, 25% downpayment.
		option.GrantAmountCents = 0
		option.LoanType = "bank"
		option.LoanAmountCents = 0
		option.InterestRateBps = 260
		option.LoanTenureMonths = 300
		option.DownpaymentPercentBps = 2500
		return
	}

	netPurchase := maxInt64(option.GetPurchasePriceCents()-option.GetGrantAmountCents(), 0)
	if netPurchase <= 0 {
		option.DownpaymentPercentBps = 0
		return
	}

	downpayment := maxInt64(netPurchase-option.GetLoanAmountCents(), 0)
	bps := int64(math.Round(float64(downpayment*10000) / float64(netPurchase)))
	if bps < 0 {
		bps = 0
	}
	if bps > 10000 {
		bps = 10000
	}
	option.DownpaymentPercentBps = bps
}

func calculateResidentialBSDCents(purchasePriceCents int64) int64 {
	amount := maxInt64(purchasePriceCents, 0)
	type tier struct {
		limitCents int64
		rateBps    int64
	}

	tiers := []tier{
		{limitCents: 18000000, rateBps: 100},
		{limitCents: 18000000, rateBps: 200},
		{limitCents: 64000000, rateBps: 300},
		{limitCents: 50000000, rateBps: 400},
		{limitCents: 150000000, rateBps: 500},
	}

	var duty int64
	for _, t := range tiers {
		if amount <= 0 {
			break
		}
		taxable := minInt64(amount, t.limitCents)
		duty += taxable * t.rateBps / 10000
		amount -= taxable
	}

	if amount > 0 {
		duty += amount * 600 / 10000
	}

	return duty
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func totalCash(people []*ourneztv1.PersonProfile) int64 {
	var total int64
	for _, person := range people {
		total += person.GetCashSavingsCents()
	}
	return total
}
