package web

import (
	ourneztv1 "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
)

func (a *App) projectedHousingTakeHomeCents(c *gin.Context, people []*ourneztv1.PersonProfile, fallback int64) int64 {
	if len(people) == 0 {
		return fallback
	}

	var total int64
	for _, person := range people {
		if person == nil {
			continue
		}

		wage := person.GetGrossMonthlyIncomeCents()
		hasFuture := person.GetExpectedFutureIncomeCents() > 0
		if hasFuture {
			wage = person.GetExpectedFutureIncomeCents()
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
