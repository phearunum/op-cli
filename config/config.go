package config

import (
	"log"
	"os"

	"ops-service/manager"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Apps []*manager.AppConfig `toml:"app"`
}

// WriteSampleConfig creates a sample pm_config.toml file.
func WriteSampleConfig() {
	sampleConfig := &Config{
		Apps: []*manager.AppConfig{
			{
				Name:    "my-app",
				Command: "node server.js",
				Port:    3000,
			},
			{
				Name:    "another-app",
				Command: "python app.py",
			},
		},
	}

	f, err := os.Create("pm_config.toml")
	if err != nil {
		log.Fatalf("Failed to create config file: %v", err)
	}
	defer f.Close()

	if err := toml.NewEncoder(f).Encode(sampleConfig); err != nil {
		log.Fatalf("Failed to encode config to file: %v", err)
	}

	log.Println("A sample 'pm_config.toml' file has been created.")
}

// LoadConfig reads the configuration file from disk.
func LoadConfig(filePath string) (*Config, error) {
	var cfg Config
	_, err := toml.DecodeFile(filePath, &cfg)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}
