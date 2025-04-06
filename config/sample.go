package config

import (
	"encoding/json"
	"errors"
	"os"

	_ "github.com/expki/go-vectorsearch/env"
)

// CreateSample creates a sample configuration file.
func CreateSample(path string) error {
	sample := Config{
		Server: ConfigServer{
			HttpAddress:  ":7500",
			HttpsAddress: ":7501",
		},
		TLS: ConfigTLS{
			DomainNameServer: []string{},
			IP:               []string{},
			Certificates:     []*ConfigTLSPath{},
		},
		AI: AI{
			Embed: Ollama{
				Url:    []string{"http://localhost:11434"},
				Model:  "nomic-embed-text",
				NumCtx: 8192,
			},
			Generate: Ollama{
				Url:    []string{"http://localhost:11434"},
				Model:  "llama3.2",
				NumCtx: 128_000,
			},
			Chat: Ollama{
				Url:    []string{"http://localhost:11434"},
				Model:  "llama3.2",
				NumCtx: 128_000,
			},
		},
		Database: Database{
			Sqlite:   "./vectorstore.db",
			Cache:    "./cache",
			LogLevel: LogLevelError,
		},
		LogLevel: LogLevelInfo,
	}
	raw, err := json.MarshalIndent(sample, "", "    ")
	if err != nil {
		return errors.Join(errors.New("could not marshal sample config"), err)
	}
	err = os.WriteFile(path, raw, 0600)
	if err != nil {
		return errors.Join(errors.New("could not write sample config file"), err)
	}
	return nil
}
