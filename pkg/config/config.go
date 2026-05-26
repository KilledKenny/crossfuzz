package config

import (
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is the top-level campaign configuration.
type Config struct {
	Campaign    CampaignConfig    `toml:"campaign"`
	Corpus      CorpusConfig      `toml:"corpus"`
	Targets     []TargetConfig    `toml:"target"`
	Comparator  ComparatorConfig  `toml:"comparator"`
	InputFilter InputFilterConfig `toml:"input_filter"`
}

// InputFilterConfig configures an optional external input filter process.
// The filter receives each generated input via shared memory and responds
// with accept or reject before the input is sent to any fuzz target.
type InputFilterConfig struct {
	Binary    string   `toml:"binary"`
	Args      []string `toml:"args"`
	BuildCmd  string   `toml:"build_cmd"`
	Env       []string `toml:"env"`
	Transform bool     `toml:"transform"`
}

// CampaignConfig controls the fuzzing campaign.
type CampaignConfig struct {
	Name         string   `toml:"name"`
	Timeout      Duration `toml:"timeout"`
	ExecTimeout  Duration `toml:"exec_timeout"`
	MaxInputSize int      `toml:"max_input_size"`
	// DictFile is an optional path to an AFL-format token dictionary fed to
	// the mutator's dict_overwrite/dict_insert strategies.
	DictFile string `toml:"dict_file"`
	// Dicts is an optional inline list of dictionary tokens, applied in
	// addition to DictFile and any comparator-derived defaults.
	Dicts []string `toml:"dicts"`
	// WarmupRounds runs the full corpus through worker 0 this many times
	// before the main fuzzing loop, stabilising flaky coverage edges.
	WarmupRounds int `toml:"warmup_rounds"`
}

// CorpusConfig specifies corpus directories.
type CorpusConfig struct {
	SeedDir     string `toml:"seed_dir"`
	CorpusDir   string `toml:"corpus_dir"`
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
	Type     string   `toml:"type"` // "harness" (default) | "server"
}

// IsServer reports whether this target is a long-running server (no pipe protocol).
func (t *TargetConfig) IsServer() bool { return t.Type == "server" }

// ComparatorConfig selects the output comparison strategy.
type ComparatorConfig struct {
	Type     string   `toml:"type"`
	Script   string   `toml:"script"`
	Binary   string   `toml:"binary"`
	Args     []string `toml:"args"`
	BuildCmd string   `toml:"build_cmd"`
	Env      []string `toml:"env"`
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
	if err := ValidateServerFuzzConfig(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ValidateServerFuzzConfig checks that server fuzz configs have exactly one harness target.
// All-harness configs (no server targets) are always valid.
func ValidateServerFuzzConfig(cfg *Config) error {
	harness, server := 0, 0
	for _, t := range cfg.Targets {
		switch t.Type {
		case "", "harness":
			harness++
		case "server":
			server++
		default:
			return fmt.Errorf("target %q: unknown type %q (want \"harness\" or \"server\")", t.Name, t.Type)
		}
	}
	if server > 0 && harness != 1 {
		return fmt.Errorf("server fuzz mode requires exactly one harness target, got %d", harness)
	}
	return nil
}
