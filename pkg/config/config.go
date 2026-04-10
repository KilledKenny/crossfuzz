package config

import (
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is the top-level campaign configuration.
type Config struct {
	Campaign   CampaignConfig   `toml:"campaign"`
	Corpus     CorpusConfig     `toml:"corpus"`
	Targets    []TargetConfig   `toml:"target"`
	Comparator ComparatorConfig `toml:"comparator"`
}

// CampaignConfig controls the fuzzing campaign.
type CampaignConfig struct {
	Name         string   `toml:"name"`
	Timeout      Duration `toml:"timeout"`
	ExecTimeout  Duration `toml:"exec_timeout"`
	MaxInputSize int      `toml:"max_input_size"`
}

// CorpusConfig specifies corpus directories.
type CorpusConfig struct {
	SeedDir     string `toml:"seed_dir"`
	CacheDir    string `toml:"cache_dir"`
	FindingsDir string `toml:"findings_dir"`
}

// TargetConfig describes one fuzz target.
type TargetConfig struct {
	Name     string   `toml:"name"`
	Language string   `toml:"language"`
	Binary   string   `toml:"binary"`
	Args     []string `toml:"args"`
	BuildCmd string   `toml:"build_cmd"`
	Env      []string `toml:"env"`
}

// ComparatorConfig selects the output comparison strategy.
type ComparatorConfig struct {
	Type   string `toml:"type"`
	Script string `toml:"script"`
}

// Duration wraps time.Duration for TOML string unmarshaling.
type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalText(text []byte) error {
	var err error
	d.Duration, err = time.ParseDuration(string(text))
	return err
}

// Load reads and parses a TOML config file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Campaign.MaxInputSize == 0 {
		cfg.Campaign.MaxInputSize = 4096
	}
	if cfg.Campaign.ExecTimeout.Duration == 0 {
		cfg.Campaign.ExecTimeout.Duration = time.Second
	}
	if cfg.Campaign.Timeout.Duration == 0 {
		cfg.Campaign.Timeout.Duration = time.Hour
	}
	if cfg.Corpus.CacheDir == "" {
		cfg.Corpus.CacheDir = "./cache"
	}
	if cfg.Corpus.FindingsDir == "" {
		cfg.Corpus.FindingsDir = "./findings"
	}
	return &cfg, nil
}
