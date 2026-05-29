package web

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	ourneztv1 "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
)

type dashboardData struct {
	FamilyID                       string
	Families                       []*ourneztv1.Family
	Dashboard                      *ourneztv1.HouseholdDashboard
	People                         []*ourneztv1.PersonProfile
	Housing                        []*ourneztv1.HousingOption
	Income                         *ourneztv1.HouseholdIncomeSummary
	HousingCharts                  []dashboardHousingChartRow
	UsesProjected                  bool
	IncomeCurrentYear              int
	IncomeProjectedYear            int
	IncomeTrendLabels              []string
	IncomeTrendGrossCents          []int64
	IncomeTrendProjectedGrossCents []int64
	TimelineSeries                 []dashboardTimelineSeries
}

type dashboardTimelineSeries struct {
	HousingID              string
	HousingName            string
	YearNow                int
	YearAssessment         int
	YearPostKeyCollection  int
	EstimatedTakeHomeCents int64
	PaymentNeedCents       int64
}

type dashboardHousingChartRow struct {
	Name                     string
	MonthlyHousingCostCents  int64
	MonthlySurplusAfterCents int64
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
	if incomeResp == nil {
		incomeResp = &ourneztv1.HouseholdIncomeSummary{}
	}
	planningTakeHome := a.projectedHousingTakeHomeCents(c, peopleResp.GetPeople(), incomeResp.GetTakeHomeIncomeCents())
	historyEntries := []*ourneztv1.IncomeHistoryEntry(nil)
	historyResp, historyErr := a.clients.Person.ListIncomeHistoryByFamily(a.grpcContext(c), &ourneztv1.ListIncomeHistoryByFamilyRequest{
		ViewerUserId: user.ID,
		FamilyId:     familyID,
	})
	if historyErr == nil && historyResp != nil {
		historyEntries = historyResp.GetEntries()
	}
	incomeTrendLabels, incomeTrendGross := buildIncomeTrendSeries(historyEntries, incomeResp.GetCurrentGrossIncomeCents(), time.Now())

	currentYear := time.Now().Year()
	projectedIncomeYear := inferProjectedIncomeYear(housingResp.GetHousingOptions(), currentYear)
	incomeTrendLabels, incomeTrendGross, incomeTrendProjectedGross := mergeProjectedPointIntoTrend(
		incomeTrendLabels,
		incomeTrendGross,
		projectedIncomeYear,
		incomeResp.GetCurrentGrossIncomeCents(),
		incomeResp.GetProjectedGrossIncomeCents(),
		time.Now(),
	)
	housingCharts := make([]dashboardHousingChartRow, 0, len(housingResp.GetHousingOptions()))
	timelineSeries := make([]dashboardTimelineSeries, 0, len(housingResp.GetHousingOptions()))
	for i, option := range housingResp.GetHousingOptions() {
		aff, affErr := a.clients.Housing.CalculateHousingAffordability(a.grpcContext(c), &ourneztv1.CalculateHousingAffordabilityRequest{
			HousingOption:        option,
			CashSavingsCents:     totalCash(peopleResp.GetPeople()),
			CpfOaCents:           incomeResp.GetCurrentCpfOaCents(),
			TakeHomeCents:        planningTakeHome,
			MonthlyExpensesCents: incomeResp.GetMonthlyExpensesCents(),
		})
		if affErr != nil {
			continue
		}
		housingCharts = append(housingCharts, dashboardHousingChartRow{
			Name:                     option.GetName(),
			MonthlyHousingCostCents:  aff.GetMonthlyHousingCostCents(),
			MonthlySurplusAfterCents: aff.GetMonthlySurplusAfterHousingCents(),
		})
		assessmentYear, postYear := timelineTransitionYears(option.GetExpectedKeyCollectionDate(), currentYear)
		housingID := strings.TrimSpace(option.GetId())
		if housingID == "" {
			housingID = "housing_option_" + strconv.Itoa(i+1)
		}
		timelineSeries = append(timelineSeries, dashboardTimelineSeries{
			HousingID:              housingID,
			HousingName:            option.GetName(),
			YearNow:                currentYear,
			YearAssessment:         assessmentYear,
			YearPostKeyCollection:  postYear,
			EstimatedTakeHomeCents: planningTakeHome,
			PaymentNeedCents:       aff.GetMonthlyHousingCostCents(),
		})
	}

	a.render(c, "dashboard", "Dashboard", dashboardData{
		FamilyID:                       familyID,
		Families:                       families,
		Dashboard:                      dResp,
		People:                         peopleResp.GetPeople(),
		Housing:                        housingResp.GetHousingOptions(),
		Income:                         incomeResp,
		HousingCharts:                  housingCharts,
		UsesProjected:                  planningTakeHome > incomeResp.GetTakeHomeIncomeCents(),
		IncomeCurrentYear:              currentYear,
		IncomeProjectedYear:            projectedIncomeYear,
		IncomeTrendLabels:              incomeTrendLabels,
		IncomeTrendGrossCents:          incomeTrendGross,
		IncomeTrendProjectedGrossCents: incomeTrendProjectedGross,
		TimelineSeries:                 timelineSeries,
	})
}

type incomeTrendPoint struct {
	RecordedAt time.Time
	PersonID   string
	GrossCents int64
}

func buildIncomeTrendSeries(entries []*ourneztv1.IncomeHistoryEntry, currentGrossCents int64, now time.Time) ([]string, []int64) {
	points := make([]incomeTrendPoint, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		recordedAt := strings.TrimSpace(entry.GetRecordedAt())
		if recordedAt == "" {
			continue
		}

		parsed, err := time.Parse(time.RFC3339, recordedAt)
		if err != nil {
			parsed, err = time.Parse(time.RFC3339Nano, recordedAt)
			if err != nil {
				continue
			}
		}

		personID := strings.TrimSpace(entry.GetPersonId())
		if personID == "" {
			continue
		}

		grossCents := entry.GetGrossMonthlyIncomeCents()
		if grossCents < 0 {
			grossCents = 0
		}

		points = append(points, incomeTrendPoint{
			RecordedAt: parsed,
			PersonID:   personID,
			GrossCents: grossCents,
		})
	}

	sort.Slice(points, func(i, j int) bool {
		return points[i].RecordedAt.Before(points[j].RecordedAt)
	})

	labels := make([]string, 0, len(points)+1)
	values := make([]int64, 0, len(points)+1)
	lastGrossByPerson := make(map[string]int64)
	var householdTotal int64
	lastMonthKey := ""
	for _, point := range points {
		previousGross := lastGrossByPerson[point.PersonID]
		householdTotal = householdTotal - previousGross + point.GrossCents
		lastGrossByPerson[point.PersonID] = point.GrossCents

		monthKey := point.RecordedAt.Format("2006-01")
		monthLabel := point.RecordedAt.Format("Jan 2006")
		if monthKey == lastMonthKey && len(values) > 0 {
			values[len(values)-1] = householdTotal
			continue
		}

		lastMonthKey = monthKey
		labels = append(labels, monthLabel)
		values = append(values, householdTotal)
	}

	nowLabel := now.Format("Jan 2006")
	if len(labels) == 0 {
		return []string{nowLabel}, []int64{maxInt64(currentGrossCents, 0)}
	}
	if labels[len(labels)-1] == nowLabel {
		values[len(values)-1] = maxInt64(currentGrossCents, 0)
		return labels, values
	}

	return append(labels, nowLabel), append(values, maxInt64(currentGrossCents, 0))
}

func mergeProjectedPointIntoTrend(labels []string, grossSeries []int64, projectedYear int, currentGrossCents int64, projectedGrossCents int64, now time.Time) ([]string, []int64, []int64) {
	if projectedYear < now.Year() {
		projectedYear = now.Year()
	}
	projectedLabel := time.Date(projectedYear, now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("Jan 2006")
	nowLabel := now.Format("Jan 2006")
	safeCurrent := maxInt64(currentGrossCents, 0)
	safeProjected := maxInt64(projectedGrossCents, 0)

	outLabels := append([]string(nil), labels...)
	outGross := append([]int64(nil), grossSeries...)
	outProjected := make([]int64, len(outLabels))
	for i := range outProjected {
		outProjected[i] = -1
	}

	projectedIndex := -1
	for i, label := range outLabels {
		if label == projectedLabel {
			projectedIndex = i
			break
		}
	}
	if projectedIndex == -1 {
		outLabels = append(outLabels, projectedLabel)
		outGross = append(outGross, maxInt64(currentGrossCentsOrLast(outGross), 0))
		outProjected = append(outProjected, safeProjected)
	} else {
		outProjected[projectedIndex] = safeProjected
	}

	currentIndex := -1
	for i, label := range outLabels {
		if label == nowLabel {
			currentIndex = i
			break
		}
	}
	if currentIndex == -1 && len(outLabels) > 0 {
		currentIndex = len(outLabels) - 1
	}
	if currentIndex >= 0 {
		outProjected[currentIndex] = safeCurrent
	}

	return outLabels, outGross, outProjected
}

func currentGrossCentsOrLast(series []int64) int64 {
	if len(series) == 0 {
		return 0
	}
	last := series[len(series)-1]
	if last < 0 {
		return 0
	}
	return last
}

func inferProjectedIncomeYear(housing []*ourneztv1.HousingOption, currentYear int) int {
	defaultProjectedYear := currentYear + 4
	if len(housing) == 0 {
		return defaultProjectedYear
	}

	earliestAssessmentYear := 0
	earliestBTOKeyYear := 0
	for _, option := range housing {
		if option == nil {
			continue
		}

		keyDate := strings.TrimSpace(option.GetExpectedKeyCollectionDate())
		if keyDate == "" || !isISODate(keyDate) {
			continue
		}

		keyParsed, keyErr := time.Parse("2006-01-02", keyDate)
		if keyErr == nil {
			keyYear := keyParsed.Year()
			if keyYear < currentYear {
				keyYear = currentYear
			}
			if eqFold(option.GetHousingType(), "bto") && (earliestBTOKeyYear == 0 || keyYear < earliestBTOKeyYear) {
				earliestBTOKeyYear = keyYear
			}
		}

		assessmentDate := assessmentDateFromKeyCollection(keyDate)
		if assessmentDate == "" || !isISODate(assessmentDate) {
			continue
		}
		assessmentParsed, assessmentErr := time.Parse("2006-01-02", assessmentDate)
		if assessmentErr != nil {
			continue
		}
		assessmentYear := assessmentParsed.Year()
		if assessmentYear < currentYear {
			assessmentYear = currentYear
		}
		if earliestAssessmentYear == 0 || assessmentYear < earliestAssessmentYear {
			earliestAssessmentYear = assessmentYear
		}
	}

	if earliestAssessmentYear != 0 {
		return earliestAssessmentYear
	}
	if earliestBTOKeyYear != 0 {
		return earliestBTOKeyYear
	}
	return defaultProjectedYear
}

func timelineTransitionYears(expectedKeyCollectionDate string, fallbackNowYear int) (assessmentYear int, postKeyYear int) {
	nowYear := fallbackNowYear
	assessmentYear = nowYear
	postKeyYear = nowYear + 1

	keyDate := strings.TrimSpace(expectedKeyCollectionDate)
	if keyDate == "" || !isISODate(keyDate) {
		return assessmentYear, postKeyYear
	}

	parsedKeyDate, err := time.Parse("2006-01-02", keyDate)
	if err != nil {
		return assessmentYear, postKeyYear
	}

	postKeyYear = parsedKeyDate.Year()
	if postKeyYear < nowYear {
		postKeyYear = nowYear
	}

	assessmentDate := assessmentDateFromKeyCollection(keyDate)
	if assessmentDate == "" || !isISODate(assessmentDate) {
		assessmentYear = nowYear
	} else if parsedAssessmentDate, parseAssessmentErr := time.Parse("2006-01-02", assessmentDate); parseAssessmentErr == nil {
		assessmentYear = parsedAssessmentDate.Year()
		if assessmentYear < nowYear {
			assessmentYear = nowYear
		}
	}

	if postKeyYear < assessmentYear {
		postKeyYear = assessmentYear
	}

	return assessmentYear, postKeyYear
}
