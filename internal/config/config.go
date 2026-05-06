package config

import (
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	DatabaseURL string `yaml:"database_url"`
	RPCURL      string `yaml:"rpc_url"`
	Workers     int    `yaml:"workers"`
	BatchSize   int    `yaml:"batch_size"`
	StartHeight int32  `yaml:"start_height"`
}

func Load(path string) (*Config, error) {
	cfg := &Config{
		Workers:   10,
		BatchSize: 100,
	}

	// Try to load from YAML if it exists
	if _, err := os.Stat(path); err == nil {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	}

	// Override with environment variables if they exist
	if env := os.Getenv("DATABASE_URL"); env != "" {
		cfg.DatabaseURL = env
	}
	if env := os.Getenv("RPC_URL"); env != "" {
		cfg.RPCURL = env
	}
	if env := os.Getenv("WORKERS"); env != "" {
		if val, err := strconv.Atoi(env); err == nil {
			cfg.Workers = val
		}
	}
	if env := os.Getenv("BATCH_SIZE"); env != "" {
		if val, err := strconv.Atoi(env); err == nil {
			cfg.BatchSize = val
		}
	}
	if env := os.Getenv("START_HEIGHT"); env != "" {
		if val, err := strconv.Atoi(env); err == nil {
			cfg.StartHeight = int32(val)
		}
	}

	return cfg, nil
}
