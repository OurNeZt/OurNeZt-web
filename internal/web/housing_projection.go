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
			projected := override.GetProjectedIncomeCents()
			if projected < 0 {
				projected = 0
			}
			overrideByPersonID[personID] = projected
			overrideByPersonID[strings.ToLower(personID)] = projected
		}
	}

	var total int64
	for _, person := range people {
		if person == nil {
			continue
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

		hasFuture := false
		if person.GetExpectedFutureIncomeCents() > 0 {
			hasFuture = true
		}
		if value, ok := overrideByPersonID[personID]; ok && value > 0 {
			hasFuture = true
		}
		if value, ok := overrideByPersonID[strings.ToLower(personID)]; ok && value > 0 {
			hasFuture = true
		}
		if hasFuture {
			// wage has already been selected from override or profile future income.
		}
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
