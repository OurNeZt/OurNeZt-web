package web

import (
	"strings"

	ourneztv1 "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
)

func (a *App) projectedHousingTakeHomeCents(c *gin.Context, people []*ourneztv1.PersonProfile, option *ourneztv1.HousingOption, fallback int64) int64 {
	if len(people) == 0 {
		return fallback
	}

	overrideByPersonID := housingProjectedIncomeOverrideIndex(option)

	var total int64
	for _, person := range people {
		if person == nil {
			continue
		}

		wage, hasFuture := resolvedProjectedHousingWage(person, overrideByPersonID)
		if wage <= 0 {
			continue
		}

		status := projectedEmploymentStatus(person.GetEmploymentStatus(), hasFuture)
		cpfResp, cpfErr := a.clients.CPF.CalculateCPFContribution(a.grpcContext(c), &ourneztv1.CalculateCPFContributionRequest{
			Age:              person.GetAge(),
			MonthlyWageCents: wage,
			EmploymentStatus: status,
		})
		if cpfErr != nil || cpfResp == nil {
			total += wage
			continue
		}

		takeHome := wage - cpfResp.GetEmployeeCents()
		if takeHome < 0 {
			takeHome = 0
		}
		total += takeHome
	}

	if total <= 0 {
		return fallback
	}
	return total
}

func (a *App) projectedHousingGrossIncomeCents(people []*ourneztv1.PersonProfile, option *ourneztv1.HousingOption, fallback int64) int64 {
	if len(people) == 0 {
		return fallback
	}

	overrideByPersonID := housingProjectedIncomeOverrideIndex(option)
	var total int64
	for _, person := range people {
		if person == nil {
			continue
		}

		wage, _ := resolvedProjectedHousingWage(person, overrideByPersonID)
		if wage <= 0 {
			continue
		}
		total += wage
	}

	if total <= 0 {
		return fallback
	}
	return total
}

func housingProjectedIncomeOverrideIndex(option *ourneztv1.HousingOption) map[string]int64 {
	overrideByPersonID := make(map[string]int64)
	if option == nil {
		return overrideByPersonID
	}

	for _, override := range option.GetDiaIncomeOverrides() {
		if override == nil {
			continue
		}
		personID := strings.TrimSpace(override.GetPersonId())
		if personID == "" {
			continue
		}
		projected := override.GetProjectedIncomeCents()
		if projected < 0 {
			projected = 0
		}
		overrideByPersonID[personID] = projected
		overrideByPersonID[strings.ToLower(personID)] = projected
	}
	return overrideByPersonID
}

func resolvedProjectedHousingWage(person *ourneztv1.PersonProfile, overrideByPersonID map[string]int64) (int64, bool) {
	if person == nil {
		return 0, false
	}

	wage := person.GetGrossMonthlyIncomeCents()
	personID := strings.TrimSpace(person.GetId())
	if value, ok := overrideByPersonID[personID]; ok {
		wage = value
	} else if value, ok := overrideByPersonID[strings.ToLower(personID)]; ok {
		wage = value
	} else if person.GetExpectedFutureIncomeCents() > 0 {
		wage = person.GetExpectedFutureIncomeCents()
	}

	hasFuture := person.GetExpectedFutureIncomeCents() > 0
	if value, ok := overrideByPersonID[personID]; ok && value > 0 {
		hasFuture = true
	}
	if value, ok := overrideByPersonID[strings.ToLower(personID)]; ok && value > 0 {
		hasFuture = true
	}
	return wage, hasFuture
}

func projectedEmploymentStatus(current string, hasFutureIncome bool) string {
	status := normalizeLookup(current)
	if !hasFutureIncome {
		return status
	}

	switch status {
	case "student", "full_time_nsf", "unemployed", "future_employee":
		return "full_time_employee"
	default:
		return status
	}
}
