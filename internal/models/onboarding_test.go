package models

import (
	"slices"
	"testing"
)

func TestOnboardingStepsOf(t *testing.T) {
	if got := OnboardingStepsOf(true); !slices.Equal(got, OnboardingSteps) {
		t.Errorf("with the badge: got %v", got)
	}
	got := OnboardingStepsOf(false)
	if slices.Contains(got, OnboardingBadge) || len(got) != len(OnboardingSteps)-1 {
		t.Errorf("without the badge: got %v", got)
	}
	if !slices.Contains(OnboardingSteps, OnboardingBadge) {
		t.Error("OnboardingStepsOf changed OnboardingSteps")
	}
}
