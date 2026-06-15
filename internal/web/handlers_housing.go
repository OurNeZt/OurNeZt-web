package web

import (
	"math"
	"net/http"
	"strings"
	"time"

	ourneztv1 "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
)

type housingData struct {
	FamilyID string
	Families []*ourneztv1.Family
	Housing  []*ourneztv1.HousingOption
}

type housingFormData struct {
	FamilyID               string
	Housing                *ourneztv1.HousingOption
	IsEdit                 bool
	AssessmentMode         string
	People                 []*ourneztv1.PersonProfile
	DIAIncomeDefaultInputs map[string]string
	GrantAmountEstimates   map[string]int64
}

type housingDetailData struct {
	FamilyID                          string
	Housing                           *ourneztv1.HousingOption
	Affordability                     *ourneztv1.HousingAffordability
	EffectiveLoanAmountCents          int64
	Inputs                            housingEstimateInputs
	AssessmentModeLabel               string
	DownpaymentPlanningNote           string
	TotalInitialPaymentCents          int64
	TotalInitialPaymentCPFOACents     int64
	TotalInitialPaymentCashCents      int64
	RemainingDownpaymentLaterCents    int64
	AvailableDownpaymentFundsCents    int64
	TotalInitialPaymentCovered        bool
	TotalInitialPaymentShortfallCents int64
}

type housingCompareData struct {
	FamilyID string
	Families []*ourneztv1.Family
	Rows     []housingCompareRow
}

type housingCompareRow struct {
	Name                     string
	Affordability            *ourneztv1.HousingAffordability
	TotalInitialPaymentCents int64
}

type housingEstimateInputs struct {
	GrossIncomeUsedCents     int64
	TakeHomeUsedCents        int64
	CPFOAUsedCents           int64
	CashSavingsUsedCents     int64
	MonthlyExpensesUsedCents int64
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
	familyID := strings.TrimSpace(c.Query("family_id"))
	people := a.listFamilyPeople(c, familyID)
	grantAmountEstimates := a.housingGrantAmountEstimates(c, familyID)

	a.render(c, "housing_form", "New Housing", housingFormData{
		FamilyID:               familyID,
		Housing:                &ourneztv1.HousingOption{},
		AssessmentMode:         "standard",
		People:                 people,
		DIAIncomeDefaultInputs: buildDIAIncomeDefaultInputs(people, nil),
		GrantAmountEstimates:   grantAmountEstimates,
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
	option.DiaIncomeOverrides = buildDIAIncomeOverridesFromForm(c, assessmentMode)

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
	inputs := housingEstimateInputs{}
	if familyID != "" {
		peopleResp, peopleErr := a.clients.Person.ListPersonProfilesByFamily(a.grpcContext(c), &ourneztv1.ListPersonProfilesByFamilyRequest{ViewerUserId: user.ID, FamilyId: familyID})
		if peopleErr == nil {
			summary, sumErr := a.clients.Income.CalculateHouseholdIncomeSummary(a.grpcContext(c), &ourneztv1.CalculateHouseholdIncomeSummaryRequest{People: peopleResp.GetPeople()})
			if sumErr == nil && summary != nil {
				planningTakeHome := a.projectedHousingTakeHomeCents(c, peopleResp.GetPeople(), option, summary.GetTakeHomeIncomeCents())
				inputs = housingEstimateInputs{
					GrossIncomeUsedCents:     a.projectedHousingGrossIncomeCents(peopleResp.GetPeople(), option, summary.GetCurrentGrossIncomeCents()),
					TakeHomeUsedCents:        planningTakeHome,
					CPFOAUsedCents:           summary.GetCurrentCpfOaCents(),
					CashSavingsUsedCents:     totalCash(peopleResp.GetPeople()),
					MonthlyExpensesUsedCents: summary.GetMonthlyExpensesCents(),
				}
				affResp, affErr := a.clients.Housing.CalculateHousingAffordability(a.grpcContext(c), &ourneztv1.CalculateHousingAffordabilityRequest{
					HousingOption:        option,
					CashSavingsCents:     inputs.CashSavingsUsedCents,
					CpfOaCents:           summary.GetCurrentCpfOaCents(),
					TakeHomeCents:        planningTakeHome,
					MonthlyExpensesCents: summary.GetMonthlyExpensesCents(),
				})
				if affErr == nil {
					aff = affResp
				}
			}
		}
	}

	requiredDownpaymentCents := aff.GetRequiredDownpaymentCents()
	effectiveLoanAmountCents := effectiveHousingLoanAmountCents(option, aff)
	totalInitialPaymentDueNowCents := totalInitialPaymentCents(option, aff)
	totalInitialPaymentCPFOACents := minInt64(maxInt64(inputs.CPFOAUsedCents, 0), totalInitialPaymentDueNowCents)
	totalInitialPaymentCashCents := maxInt64(totalInitialPaymentDueNowCents-totalInitialPaymentCPFOACents, 0)
	remainingDownpaymentLaterCents := maxInt64(requiredDownpaymentCents-initialDownpaymentCents(option, aff), 0)
	if aff.GetFinalDownpaymentCents() > 0 {
		remainingDownpaymentLaterCents = maxInt64(aff.GetFinalDownpaymentCents(), 0)
	}
	availableDownpaymentFundsCents := maxInt64(inputs.CPFOAUsedCents, 0) + maxInt64(inputs.CashSavingsUsedCents, 0)
	totalInitialPaymentShortfallCents := maxInt64(totalInitialPaymentDueNowCents-availableDownpaymentFundsCents, 0)

	a.render(c, "housing_detail", "Housing Detail", housingDetailData{
		FamilyID:                          familyID,
		Housing:                           option,
		Affordability:                     aff,
		EffectiveLoanAmountCents:          effectiveLoanAmountCents,
		Inputs:                            inputs,
		AssessmentModeLabel:               housingAssessmentModeLabel(option),
		DownpaymentPlanningNote:           housingDownpaymentPlanningNote(option),
		TotalInitialPaymentCents:          totalInitialPaymentDueNowCents,
		TotalInitialPaymentCPFOACents:     totalInitialPaymentCPFOACents,
		TotalInitialPaymentCashCents:      totalInitialPaymentCashCents,
		RemainingDownpaymentLaterCents:    remainingDownpaymentLaterCents,
		AvailableDownpaymentFundsCents:    availableDownpaymentFundsCents,
		TotalInitialPaymentCovered:        totalInitialPaymentShortfallCents == 0,
		TotalInitialPaymentShortfallCents: totalInitialPaymentShortfallCents,
	})
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
	people := a.listFamilyPeople(c, familyID)
	grantAmountEstimates := a.housingGrantAmountEstimates(c, familyID)
	if inferHousingAssessmentMode(resp) != "deferred" {
		if estimate, ok := grantAmountEstimates[normalizeLookup(resp.GetHousingType())]; ok {
			resp.GrantAmountCents = estimate
		}
	}
	a.render(c, "housing_form", "Edit Housing", housingFormData{
		FamilyID:               familyID,
		Housing:                resp,
		IsEdit:                 true,
		AssessmentMode:         inferHousingAssessmentMode(resp),
		People:                 people,
		DIAIncomeDefaultInputs: buildDIAIncomeDefaultInputs(people, resp),
		GrantAmountEstimates:   grantAmountEstimates,
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
	option.DiaIncomeOverrides = buildDIAIncomeOverridesFromForm(c, assessmentMode)

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
	if summary == nil {
		summary = &ourneztv1.HouseholdIncomeSummary{}
	}
	rows := make([]housingCompareRow, 0, len(hResp.GetHousingOptions()))
	for _, option := range hResp.GetHousingOptions() {
		planningTakeHome := a.projectedHousingTakeHomeCents(c, pResp.GetPeople(), option, summary.GetTakeHomeIncomeCents())
		row, rowErr := a.clients.Housing.CalculateHousingAffordability(a.grpcContext(c), &ourneztv1.CalculateHousingAffordabilityRequest{
			HousingOption:        option,
			CashSavingsCents:     totalCash(pResp.GetPeople()),
			CpfOaCents:           summary.GetCurrentCpfOaCents(),
			TakeHomeCents:        planningTakeHome,
			MonthlyExpensesCents: summary.GetMonthlyExpensesCents(),
		})
		if rowErr == nil {
			rows = append(rows, housingCompareRow{
				Name:                     option.GetName(),
				Affordability:            row,
				TotalInitialPaymentCents: totalInitialPaymentCents(option, row),
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
		LoanTenureMonths:          yearsToMonths(parseInt32(c.PostForm("loan_tenure_years"))),
		DownpaymentPercentBps:     0,
		RenovationBudgetCents:     parseMoneyCents(c.PostForm("renovation_budget")),
		FurnitureBudgetCents:      parseMoneyCents(c.PostForm("furniture_budget")),
		LegalFeesCents:            parseMoneyCents(c.PostForm("legal_fees")),
		BuyerStampDutyCents:       0,
		MonthlyMaintenanceCents:   parseMoneyCents(c.PostForm("monthly_maintenance")),
		ExpectedKeyCollectionDate: strings.TrimSpace(c.PostForm("expected_key_collection_date")),
	}
}

func yearsToMonths(years int32) int32 {
	if years <= 0 {
		return 0
	}
	return years * 12
}

func monthsToYears(months int32) int32 {
	if months <= 0 {
		return 0
	}
	return months / 12
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
		if strings.TrimSpace(option.GetExpectedKeyCollectionDate()) == "" {
			return "expected key collection date is required for DIA mode"
		}
	} else {
		switch option.GetLoanType() {
		case "hdb", "bank", "cash":
		default:
			return "loan type is required"
		}
	}
	isBtoHdb := normalizeLookup(option.GetHousingType()) == "bto" && normalizeLookup(option.GetLoanType()) == "hdb"
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
		if option.GetLoanTenureMonths() <= 0 && !isBtoHdb {
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

	loanType := normalizeLookup(option.GetLoanType())
	if loanType == "cash" {
		return "standard"
	}

	hasValidKeyDate := isISODate(option.GetExpectedKeyCollectionDate())
	looksDeferredByLegacyShape := option.GetLoanAmountCents() == 0 && option.GetDownpaymentPercentBps() == 2500
	looksDeferredByKeyDate := hasValidKeyDate &&
		option.GetLoanAmountCents() == 0 &&
		option.GetGrantAmountCents() == 0 &&
		(loanType == "" || loanType == "bank" || loanType == "hdb")
	looksDeferredByDefaultedLoan := hasValidKeyDate &&
		loanType == "bank" &&
		option.GetLoanAmountCents() == 0 &&
		option.GetLoanTenureMonths() == 300
	looksDeferredByOverrides := hasValidKeyDate && len(option.GetDiaIncomeOverrides()) > 0

	if looksDeferredByLegacyShape || looksDeferredByKeyDate || looksDeferredByDefaultedLoan || looksDeferredByOverrides {
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
		if normalizeLookup(option.GetHousingType()) == "bto" {
			option.LoanTenureMonths = 300
		}
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

	option.DiaIncomeOverrides = nil

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

func assessmentDateFromKeyCollection(keyDate string) string {
	if !isISODate(keyDate) {
		return ""
	}
	parsed, err := time.Parse("2006-01-02", keyDate)
	if err != nil {
		return ""
	}
	// DIA is evaluated in the 3-6 month window before key collection.
	// We anchor projected planning at 6 months before as a conservative baseline.
	return parsed.AddDate(0, -6, 0).Format("2006-01-02")
}

func (a *App) listFamilyPeople(c *gin.Context, familyID string) []*ourneztv1.PersonProfile {
	user := userFromContext(c)
	if user == nil || strings.TrimSpace(familyID) == "" {
		return nil
	}
	resp, err := a.clients.Person.ListPersonProfilesByFamily(a.grpcContext(c), &ourneztv1.ListPersonProfilesByFamilyRequest{
		ViewerUserId: user.ID,
		FamilyId:     familyID,
	})
	if err != nil {
		return nil
	}
	return resp.GetPeople()
}

func extractDIAIncomeInputs(c *gin.Context) map[string]string {
	result := make(map[string]string)
	if c == nil {
		return result
	}

	for key, value := range c.PostFormMap("dia_income") {
		cleanKey := strings.TrimSpace(key)
		cleanValue := strings.TrimSpace(value)
		if cleanKey == "" || cleanValue == "" {
			continue
		}
		result[cleanKey] = cleanValue
		result[strings.ToLower(cleanKey)] = cleanValue
	}

	if c.Request == nil {
		return result
	}
	_ = c.Request.ParseForm()
	for rawKey, values := range c.Request.PostForm {
		if len(values) == 0 {
			continue
		}
		value := strings.TrimSpace(values[len(values)-1])
		if value == "" {
			continue
		}
		switch {
		case strings.HasPrefix(rawKey, "dia_income[") && strings.HasSuffix(rawKey, "]"):
			personID := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(rawKey, "dia_income["), "]"))
			if personID == "" {
				continue
			}
			result[personID] = value
			result[strings.ToLower(personID)] = value
		case strings.HasPrefix(rawKey, "dia_income_"):
			personID := strings.TrimSpace(strings.TrimPrefix(rawKey, "dia_income_"))
			if personID == "" {
				continue
			}
			result[personID] = value
			result[strings.ToLower(personID)] = value
		}
	}
	return result
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

func housingAssessmentModeLabel(option *ourneztv1.HousingOption) string {
	if inferHousingAssessmentMode(option) == "deferred" {
		return "Deferred Income Assessment (DIA)"
	}
	return "Standard Income Assessment"
}

func housingDownpaymentPlanningNote(option *ourneztv1.HousingOption) string {
	if normalizeLookup(option.GetLoanType()) == "cash" {
		return "Cash purchase planning treats the full home price and buyer stamp duty as due upfront because there is no staged loan downpayment."
	}
	if inferHousingAssessmentMode(option) == "deferred" {
		return "This total initial payment estimate combines a 2.5% due-now DIA checkpoint with buyer stamp duty, while the remaining downpayment is shown separately for later payment before or at completion."
	}
	return "This total initial payment estimate combines a 5% due-now Standard checkpoint with buyer stamp duty, while the remaining downpayment is shown separately for later payment."
}

func initialDownpaymentCents(option *ourneztv1.HousingOption, aff *ourneztv1.HousingAffordability) int64 {
	if aff == nil {
		return 0
	}

	if aff.GetInitialDownpaymentCents() > 0 {
		return maxInt64(aff.GetInitialDownpaymentCents(), 0)
	}

	requiredDownpaymentCents := maxInt64(aff.GetRequiredDownpaymentCents(), 0)
	if requiredDownpaymentCents == 0 {
		return 0
	}

	if normalizeLookup(option.GetLoanType()) == "cash" {
		return requiredDownpaymentCents
	}

	initialBps := int64(500)
	if inferHousingAssessmentMode(option) == "deferred" {
		initialBps = 250
	}

	initialByAssumptionCents := centsByBps(maxInt64(option.GetPurchasePriceCents(), 0), initialBps)
	if initialByAssumptionCents <= 0 {
		return requiredDownpaymentCents
	}
	return minInt64(requiredDownpaymentCents, initialByAssumptionCents)
}

func totalInitialPaymentCents(option *ourneztv1.HousingOption, aff *ourneztv1.HousingAffordability) int64 {
	return maxInt64(initialDownpaymentCents(option, aff), 0) + effectiveBuyerStampDutyCents(option)
}

func effectiveBuyerStampDutyCents(option *ourneztv1.HousingOption) int64 {
	if option == nil {
		return 0
	}
	if option.GetBuyerStampDutyCents() > 0 {
		return maxInt64(option.GetBuyerStampDutyCents(), 0)
	}
	if option.GetPurchasePriceCents() <= 0 {
		return 0
	}
	return calculateResidentialBSDCents(option.GetPurchasePriceCents())
}

func effectiveHousingLoanAmountCents(option *ourneztv1.HousingOption, aff *ourneztv1.HousingAffordability) int64 {
	if option == nil {
		if aff == nil {
			return 0
		}
		return maxInt64(aff.GetEstimatedLoanAmountCents(), 0)
	}

	if normalizeLookup(option.GetLoanType()) == "cash" {
		return 0
	}

	if inferHousingAssessmentMode(option) == "deferred" {
		if aff != nil && aff.GetEstimatedLoanAmountCents() > 0 {
			return aff.GetEstimatedLoanAmountCents()
		}

		netPriceCents := maxInt64(option.GetPurchasePriceCents()-option.GetGrantAmountCents(), 0)
		return centsByBps(netPriceCents, 7500)
	}

	return maxInt64(option.GetLoanAmountCents(), 0)
}

func centsByBps(amountCents, bps int64) int64 {
	if amountCents <= 0 || bps <= 0 {
		return 0
	}
	return amountCents * bps / 10000
}

func buildDIAIncomeDefaultInputs(people []*ourneztv1.PersonProfile, option *ourneztv1.HousingOption) map[string]string {
	defaults := make(map[string]string, len(people))
	overrideByPersonID := make(map[string]int64)
	if option != nil {
		for _, override := range option.GetDiaIncomeOverrides() {
			if override == nil {
				continue
			}
			personID := strings.TrimSpace(override.GetPersonId())
			if personID == "" {
				continue
			}
			value := override.GetProjectedIncomeCents()
			if value < 0 {
				value = 0
			}
			overrideByPersonID[personID] = value
			overrideByPersonID[strings.ToLower(personID)] = value
		}
	}

	for _, person := range people {
		if person == nil || strings.TrimSpace(person.GetId()) == "" {
			continue
		}
		defaultIncomeCents := int64(0)
		if value, ok := overrideByPersonID[person.GetId()]; ok {
			defaultIncomeCents = value
		} else if value, ok := overrideByPersonID[strings.ToLower(person.GetId())]; ok {
			defaultIncomeCents = value
		} else {
			defaultIncomeCents = person.GetExpectedFutureIncomeCents()
		}
		if defaultIncomeCents <= 0 {
			defaultIncomeCents = person.GetGrossMonthlyIncomeCents()
		}
		if defaultIncomeCents < 0 {
			defaultIncomeCents = 0
		}
		defaults[person.GetId()] = centsInputString(defaultIncomeCents)
	}
	return defaults
}

func buildDIAIncomeOverridesFromForm(c *gin.Context, assessmentMode string) []*ourneztv1.HousingDIAIncomeOverride {
	if normalizeLookup(assessmentMode) != "deferred" {
		return nil
	}
	inputs := extractDIAIncomeInputs(c)
	if len(inputs) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(inputs))
	overrides := make([]*ourneztv1.HousingDIAIncomeOverride, 0, len(inputs))
	for personID, raw := range inputs {
		cleanPersonID := strings.TrimSpace(personID)
		if cleanPersonID == "" {
			continue
		}
		canonical := strings.ToLower(cleanPersonID)
		if _, exists := seen[canonical]; exists {
			continue
		}
		seen[canonical] = struct{}{}

		projected := parseMoneyCents(raw)
		if projected < 0 {
			projected = 0
		}
		overrides = append(overrides, &ourneztv1.HousingDIAIncomeOverride{
			PersonId:             cleanPersonID,
			ProjectedIncomeCents: projected,
		})
	}
	return overrides
}

func (a *App) housingGrantAmountEstimates(c *gin.Context, familyID string) map[string]int64 {
	result := map[string]int64{
		"bto":             0,
		"resale_hdb":      0,
		"executive_condo": 0,
		"private_condo":   0,
		"landed":          0,
		"other":           0,
	}
	user := userFromContext(c)
	if user == nil || strings.TrimSpace(familyID) == "" {
		return result
	}

	for housingType := range result {
		resp, err := a.clients.Housing.EstimateHousingGrant(a.grpcContext(c), &ourneztv1.EstimateHousingGrantRequest{
			ViewerUserId: user.ID,
			FamilyId:     familyID,
			HousingType:  housingType,
		})
		if err != nil || resp == nil {
			continue
		}
		result[housingType] = maxInt64(resp.GetGrantAmountCents(), 0)
	}
	return result
}

func totalCash(people []*ourneztv1.PersonProfile) int64 {
	var total int64
	for _, person := range people {
		total += person.GetCashSavingsCents()
	}
	return total
}
