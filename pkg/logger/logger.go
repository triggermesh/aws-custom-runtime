/*
Copyright 2022 Triggermesh Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package logger

import (
	"encoding/json"
	"os"

	"github.com/blendle/zapdriver"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	// Environment variable used by Knative to pass Zap logging config.
	EnvLoggingConfigJson = "K_LOGGING_CONFIG"
	// JSON's key that stores Zap configuration.
	ZapLoggerConfigKey = "zap-logger-config"
)

// JSON structure to access Zap configuration.
type loggingEnvConfig struct {
	ZapConfig string `json:"zap-logger-config,omitempty"`
}

// New returns sugared Zap logger.
func New() *zap.SugaredLogger {
	zapConfig := defaultProductionConfig()
	var err error

	configJSON, exists := os.LookupEnv(EnvLoggingConfigJson)
	if exists && configJSON != "" {
		if err = updateConfigFromJSON(&zapConfig, configJSON); err != nil {
			panic(err)
		}
	}

	logger, err := zapConfig.Build()
	if err != nil {
		panic(err)
	}

	return logger.Sugar()
}

// updateConfigFromJSON updates default Zap configuration with configuration passed
// in environment variable.
func updateConfigFromJSON(defaultConfig *zap.Config, configJSON string) error {
	var lc loggingEnvConfig
	if err := json.Unmarshal([]byte(configJSON), &lc); err != nil {
		return err
	}
	return json.Unmarshal([]byte(lc.ZapConfig), defaultConfig)
}

// defaultProductionConfig creates default production logger config.
func defaultProductionConfig() zap.Config {
	cfg := zapdriver.NewProductionConfig()
	cfg.EncoderConfig.EncodeDuration = zapcore.StringDurationEncoder
	return cfg
}
