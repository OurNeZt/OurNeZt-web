package web

import (
	"strings"
	"testing"

	ourneztv1 "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
)

func TestValidateHousingOptionInputRejectsLoanTenureAboveHousingTypeCap(t *testing.T) {
	tests := []struct {
		name         string
		housingType  string
		tenureMonths int32
		wantMessage  string
	}{
		{
			name:         "HDB above 30 years",
			housingType:  "bto",
			tenureMonths: 31 * 12,
			wantMessage:  "cannot exceed 30 years",
		},
		{
			name:         "non-HDB above 35 years",
			housingType:  "private_condo",
			tenureMonths: 36 * 12,
			wantMessage:  "cannot exceed 35 years",
		},
		{
			name:         "non-HDB at 35 years",
			housingType:  "private_condo",
			tenureMonths: 35 * 12,
			wantMessage:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			option := validHousingOptionForValidation()
			option.HousingType = tc.housingType
			option.LoanTenureMonths = tc.tenureMonths
			errMessage := validateHousingOptionInput(option, "standard")

			if tc.wantMessage == "" {
				if errMessage != "" {
					t.Fatalf("validation error = %q, want none", errMessage)
				}
				return
			}
			if !strings.Contains(errMessage, tc.wantMessage) {
				t.Fatalf("validation error = %q, want to contain %q", errMessage, tc.wantMessage)
			}
		})
	}
}

func TestValidateHousingOptionInputRequiresKeyCollectionDate(t *testing.T) {
	option := validHousingOptionForValidation()
	option.ExpectedKeyCollectionDate = ""

	errMessage := validateHousingOptionInput(option, "standard")
	if !strings.Contains(errMessage, "expected key collection date is required") {
		t.Fatalf("validation error = %q, want expected key collection date requirement", errMessage)
	}
}

func TestValidateHousingOptionInputRequiresBankInterestRate(t *testing.T) {
	option := validHousingOptionForValidation()
	option.InterestRateBps = 0

	errMessage := validateHousingOptionInput(option, "standard")
	if !strings.Contains(errMessage, "interest rate is required") {
		t.Fatalf("validation error = %q, want interest rate requirement", errMessage)
	}
}

func TestValidateHousingOptionInputRejectsDeferredForResaleHDB(t *testing.T) {
	option := validHousingOptionForValidation()
	option.HousingType = "resale_hdb"

	errMessage := validateHousingOptionInput(option, "deferred")
	if !strings.Contains(errMessage, "deferred income assessment is only available for BTO options") {
		t.Fatalf("validation error = %q, want BTO-only deferred requirement", errMessage)
	}
}

func TestValidateHousingOptionInputRejectsNonHDBPropertyHDBOnlyValues(t *testing.T) {
	tests := []struct {
		name        string
		housingType string
		mutate      func(*ourneztv1.HousingOption) string
		wantMessage string
	}{
		{
			name:        "EC deferred assessment",
			housingType: "executive_condo",
			mutate: func(option *ourneztv1.HousingOption) string {
				return "deferred"
			},
			wantMessage: "deferred income assessment is only available",
		},
		{
			name:        "landed grant amount",
			housingType: "landed",
			mutate: func(option *ourneztv1.HousingOption) string {
				option.GrantAmountCents = 1000000
				return "standard"
			},
			wantMessage: "grant amount is not applicable",
		},
		{
			name:        "other HDB loan",
			housingType: "other",
			mutate: func(option *ourneztv1.HousingOption) string {
				option.LoanType = "hdb"
				return "standard"
			},
			wantMessage: "HDB loan is not available",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			option := validHousingOptionForValidation()
			option.HousingType = tc.housingType
			assessmentMode := tc.mutate(option)

			errMessage := validateHousingOptionInput(option, assessmentMode)
			if !strings.Contains(errMessage, tc.wantMessage) {
				t.Fatalf("validation error = %q, want to contain %q", errMessage, tc.wantMessage)
			}
		})
	}
}

func TestBuildHousingGroupSections(t *testing.T) {
	groups := []*ourneztv1.HousingGroup{
		{Id: "group_1", Name: "June BTO 2026"},
	}
	visible := true
	hidden := false
	options := []*ourneztv1.HousingOption{
		{Id: "housing_1", Name: "Plan A", HousingGroupId: &groups[0].Id, VisibleOnDashboard: &visible},
		{Id: "housing_2", Name: "Plan B", HousingGroupId: &groups[0].Id, VisibleOnDashboard: &hidden},
		{Id: "housing_3", Name: "Plan C", VisibleOnDashboard: &visible},
	}

	sections := buildHousingGroupSections(groups, options)
	if len(sections) != 2 {
		t.Fatalf("sections len = %d, want 2", len(sections))
	}
	if sections[0].GroupName != "June BTO 2026" {
		t.Fatalf("first section name = %q, want June BTO 2026", sections[0].GroupName)
	}
	if !sections[0].MixedVisibility {
		t.Fatal("first section mixed visibility = false, want true")
	}
	if !sections[1].IsUngrouped {
		t.Fatal("second section IsUngrouped = false, want true")
	}
}

func TestVisibleHousingOptions(t *testing.T) {
	visible := true
	hidden := false
	options := []*ourneztv1.HousingOption{
		{Id: "housing_1", VisibleOnDashboard: &visible},
		{Id: "housing_2", VisibleOnDashboard: &hidden},
		{Id: "housing_3"},
	}

	filtered := visibleHousingOptions(options)
	if len(filtered) != 2 {
		t.Fatalf("filtered len = %d, want 2", len(filtered))
	}
	if filtered[0].GetId() != "housing_1" || filtered[1].GetId() != "housing_3" {
		t.Fatalf("filtered ids = [%q, %q], want [housing_1, housing_3]", filtered[0].GetId(), filtered[1].GetId())
	}
}

func TestParseBoolFormValuesUsesLastSubmittedValue(t *testing.T) {
	if !parseBoolFormValues([]string{"false", "true"}) {
		t.Fatal("parseBoolFormValues returned false, want true")
	}
	if parseBoolFormValues([]string{"true", "false"}) {
		t.Fatal("parseBoolFormValues returned true, want false")
	}
	if parseBoolFormValues(nil) {
		t.Fatal("parseBoolFormValues returned true for nil input, want false")
	}
}

func validHousingOptionForValidation() *ourneztv1.HousingOption {
	return &ourneztv1.HousingOption{
		Name:                      "Option",
		HousingType:               "private_condo",
		UnitType:                  "4-room",
		PurchasePriceCents:        45000000,
		LoanType:                  "bank",
		LoanAmountCents:           30000000,
		InterestRateBps:           260,
		LoanTenureMonths:          35 * 12,
		ExpectedKeyCollectionDate: "2028-01-01",
	}
}
