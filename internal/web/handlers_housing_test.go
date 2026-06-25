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
