package main

import (
	"testing"
)

func TestMaxTokensOverride(t *testing.T) {
	ptr := func(i int) *int { return &i }

	// Case 1: CLI override is nil (not set), should use persona or model default
	rc1 := &RunConfig{
		Settings: &RunSettings{MaxTokens: nil},
	}
	p1 := Persona{MaxTokens: ptr(500)}
	modelCfg1 := ModelConfig{MaxTokens: ptr(1000)}

	maxTokens1 := 0
	if modelCfg1.MaxTokens != nil {
		maxTokens1 = *modelCfg1.MaxTokens
	}
	if p1.MaxTokens != nil {
		maxTokens1 = *p1.MaxTokens
	}
	if rc1.Settings.MaxTokens != nil {
		maxTokens1 = *rc1.Settings.MaxTokens
	}
	if maxTokens1 != 500 {
		t.Errorf("Expected maxTokens to be 500 (persona default), got %d", maxTokens1)
	}

	// Case 2: CLI override is 0 (explicitly set to no limit), should use 0
	rc2 := &RunConfig{
		Settings: &RunSettings{MaxTokens: ptr(0)},
	}
	p2 := Persona{MaxTokens: ptr(500)}
	modelCfg2 := ModelConfig{MaxTokens: ptr(1000)}

	maxTokens2 := 0
	if modelCfg2.MaxTokens != nil {
		maxTokens2 = *modelCfg2.MaxTokens
	}
	if p2.MaxTokens != nil {
		maxTokens2 = *p2.MaxTokens
	}
	if rc2.Settings.MaxTokens != nil {
		maxTokens2 = *rc2.Settings.MaxTokens
	}
	if maxTokens2 != 0 {
		t.Errorf("Expected maxTokens to be 0 (CLI override), got %d", maxTokens2)
	}

	// Case 3: CLI override is 1500 (explicit value), should use 1500
	rc3 := &RunConfig{
		Settings: &RunSettings{MaxTokens: ptr(1500)},
	}
	p3 := Persona{MaxTokens: ptr(500)}
	modelCfg3 := ModelConfig{MaxTokens: ptr(1000)}

	maxTokens3 := 0
	if modelCfg3.MaxTokens != nil {
		maxTokens3 = *modelCfg3.MaxTokens
	}
	if p3.MaxTokens != nil {
		maxTokens3 = *p3.MaxTokens
	}
	if rc3.Settings.MaxTokens != nil {
		maxTokens3 = *rc3.Settings.MaxTokens
	}
	if maxTokens3 != 1500 {
		t.Errorf("Expected maxTokens to be 1500 (CLI override), got %d", maxTokens3)
	}

	// Case 4: Persona has 0 (explicitly set to no limit), model has limit
	rc4 := &RunConfig{
		Settings: &RunSettings{MaxTokens: nil},
	}
	p4 := Persona{MaxTokens: ptr(0)}
	modelCfg4 := ModelConfig{MaxTokens: ptr(1000)}

	maxTokens4 := 0
	if modelCfg4.MaxTokens != nil {
		maxTokens4 = *modelCfg4.MaxTokens
	}
	if p4.MaxTokens != nil {
		maxTokens4 = *p4.MaxTokens
	}
	if rc4.Settings.MaxTokens != nil {
		maxTokens4 = *rc4.Settings.MaxTokens
	}
	if maxTokens4 != 0 {
		t.Errorf("Expected maxTokens to be 0 (Persona override), got %d", maxTokens4)
	}
}
