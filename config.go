package main

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ModelMapping       map[string]ModelConfig `yaml:"model_mapping"`
	GlobalInstructions string                 `yaml:"global_instructions"`
	PrimerTypes        map[string]PrimerType  `yaml:"primer_types"`
}

type PrimerType struct {
	Description string `yaml:"description"`
}

type ModelConfig struct {
	Provider              string  `yaml:"provider"`
	Model                 string  `yaml:"model"`
	MaxTokens             int     `yaml:"max_tokens"`
	ReasoningLevel        string  `yaml:"reasoning_level,omitempty"` // none | low | medium | high
	InputPricePerMillion  float64 `yaml:"input_price_per_million"`
	OutputPricePerMillion float64 `yaml:"output_price_per_million"`
}

func LoadConfig(searchPaths []string, repo string, oh *OutputHandler) (*Config, error) {
	finalConfig := &Config{
		ModelMapping: make(map[string]ModelConfig),
		PrimerTypes:  make(map[string]PrimerType),
	}

	found := false
	for _, base := range searchPaths {
		configPath := filepath.Join(base, ".ai-review", repo, "config.yaml")
		data, err := os.ReadFile(configPath)
		if err != nil {
			// Fallback to global config if repo-specific doesn't exist?
			// User said "the tool should look in the local directory .ai-review/gonka-ai/gonka/"
			// Let's try to look in the old location as well for backward compatibility?
			// "The local .ai-review should directory should be specific to a specific owner/repo."
			// If I just change it to look in repo specific, it matches the requirement.
			configPath = filepath.Join(base, ".ai-review/config.yaml")
			data, err = os.ReadFile(configPath)
			if err != nil {
				continue
			}
		}

		oh.Printf("    -> Loading config from: %s\n", configPath)
		var cfg Config
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			oh.Printf("Warning: error parsing config at %s: %v\n", configPath, err)
			continue
		}

		found = true
		// Merge model mappings
		for k, v := range cfg.ModelMapping {
			finalConfig.ModelMapping[k] = v
		}
		// Merge primer types
		for k, v := range cfg.PrimerTypes {
			finalConfig.PrimerTypes[k] = v
		}
		if cfg.GlobalInstructions != "" {
			finalConfig.GlobalInstructions = cfg.GlobalInstructions
		}
	}

	if !found {
		return nil, os.ErrNotExist
	}

	return finalConfig, nil
}
