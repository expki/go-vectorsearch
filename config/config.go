package config

import (
	"encoding/json"
	"errors"

	_ "github.com/expki/go-vectorsearch/env"
)

// ParseConfig parses the raw JSON configuration.
func ParseConfig(raw []byte) (config Config, err error) {
	err = json.Unmarshal(raw, &config)
	if err != nil {
		return config, errors.Join(errors.New("unmarshal config"), err)
	}
	return config, nil
}

type Config struct {
	Server   ConfigServer `json:"server"`
	TLS      ConfigTLS    `json:"tls"`
	Database Database     `json:"database"`
	Ollama   Ollama       `json:"ollama"`
	Cache    string       `json:"cache"`
	LogLevel LogLevel     `json:"log_level"`
}

type ConfigServer struct {
	HttpAddress  string `json:"http_address"`
	HttpsAddress string `json:"https_address"`
}
