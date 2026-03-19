package main

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ModelDefinitions   map[string]ModelConfig            `yaml:"model_definitions"`
	ModelProfiles      map[string]map[string]ModelConfig `yaml:"model_profiles"`
	DefaultProfile     string                            `yaml:"default_profile"`
	GlobalInstructions string                            `yaml:"global_instructions"`
	PrimerTypes        map[string]PrimerType             `yaml:"primer_types"`
}

type PrimerType struct {
	Description string `yaml:"description"`
}

type ModelConfig struct {
	ID                    string  `yaml:"id,omitempty"`
	Provider              string  `yaml:"provider"`
	Model                 string  `yaml:"model"`
	MaxTokens             int     `yaml:"max_tokens"`
	ReasoningLevel        string  `yaml:"reasoning_level,omitempty"` // none | low | medium | high
	InputPricePerMillion  float64 `yaml:"input_price_per_million"`
	OutputPricePerMillion float64 `yaml:"output_price_per_million"`
}

func LoadConfig(searchPaths []string, repo string, oh *OutputHandler) (*Config, error) {
	finalConfig := &Config{
		ModelDefinitions: make(map[string]ModelConfig),
		ModelProfiles:    make(map[string]map[string]ModelConfig),
		PrimerTypes:      make(map[string]PrimerType),
	}

	// 1. Load base models.yaml if it exists in any search path
	for _, base := range searchPaths {
		for _, dirName := range []string{".ai-review", ".ai-reviewer"} {
			modelsPath := filepath.Join(base, dirName, "models.yaml")
			if data, err := os.ReadFile(modelsPath); err == nil {
				oh.Printf("    -> Loading base models from: %s\n", modelsPath)
				var defs struct {
					ModelDefinitions map[string]ModelConfig `yaml:"model_definitions"`
				}
				if err := yaml.Unmarshal(data, &defs); err != nil {
					oh.Printf("Warning: error parsing models at %s: %v\n", modelsPath, err)
				} else {
					for k, v := range defs.ModelDefinitions {
						finalConfig.ModelDefinitions[k] = v
					}
				}
			}
		}
	}

	found := false
	for _, base := range searchPaths {
		var configPath string
		var configDir string
		var data []byte
		var err error

		for _, dirName := range []string{".ai-review", ".ai-reviewer"} {
			configDir = filepath.Join(base, dirName, repo)
			configPath = filepath.Join(configDir, "config.yaml")
			data, err = os.ReadFile(configPath)
			if err == nil {
				break
			}
			configDir = filepath.Join(base, dirName)
			configPath = filepath.Join(configDir, "config.yaml")
			data, err = os.ReadFile(configPath)
			if err == nil {
				break
			}
		}

		if err != nil {
			continue
		}

		// Look for models.yaml in the same directory as config.yaml
		modelsPath := filepath.Join(configDir, "models.yaml")
		if mdata, err := os.ReadFile(modelsPath); err == nil {
			oh.Printf("    -> Loading models override from: %s\n", modelsPath)
			var defs struct {
				ModelDefinitions map[string]ModelConfig `yaml:"model_definitions"`
			}
			if err := yaml.Unmarshal(mdata, &defs); err != nil {
				oh.Printf("Warning: error parsing models at %s: %v\n", modelsPath, err)
			} else {
				for k, v := range defs.ModelDefinitions {
					finalConfig.ModelDefinitions[k] = v
				}
			}
		}

		oh.Printf("    -> Loading config from: %s\n", configPath)
		var cfg Config
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			oh.Printf("Warning: error parsing config at %s: %v\n", configPath, err)
			continue
		}

		found = true
		// Merge model definitions
		for k, v := range cfg.ModelDefinitions {
			finalConfig.ModelDefinitions[k] = v
		}

		// Merge model profiles
		for profileName, profileMapping := range cfg.ModelProfiles {
			if _, ok := finalConfig.ModelProfiles[profileName]; !ok {
				finalConfig.ModelProfiles[profileName] = make(map[string]ModelConfig)
			}
			for k, v := range profileMapping {
				// Resolve model definition if ID is present
				if v.ID != "" {
					if def, ok := finalConfig.ModelDefinitions[v.ID]; ok {
						// Use values from definition if not overridden in v
						if v.Provider == "" {
							v.Provider = def.Provider
						}
						if v.Model == "" {
							v.Model = def.Model
						}
						if v.MaxTokens == 0 {
							v.MaxTokens = def.MaxTokens
						}
						if v.ReasoningLevel == "" {
							v.ReasoningLevel = def.ReasoningLevel
						}
						if v.InputPricePerMillion == 0 {
							v.InputPricePerMillion = def.InputPricePerMillion
						}
						if v.OutputPricePerMillion == 0 {
							v.OutputPricePerMillion = def.OutputPricePerMillion
						}
					} else {
						oh.Printf("Warning: model definition '%s' not found for profile '%s' in %s\n", v.ID, profileName, configPath)
					}
				}
				finalConfig.ModelProfiles[profileName][k] = v
			}
		}
		if cfg.DefaultProfile != "" {
			finalConfig.DefaultProfile = cfg.DefaultProfile
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
