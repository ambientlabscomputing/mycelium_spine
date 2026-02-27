package utils

import (
	"fmt"
	"log/slog"
	"os"
	"reflect"

	"gopkg.in/yaml.v3"
)

type SecretString string

func (s SecretString) String() string {
	return "****"
}

type Settings struct {
	// Application logging
	LogLevel    string `yaml:"log_level"`
	LogFormat   string `yaml:"log_format"`
	LogToStderr bool   `yaml:"log_to_stderr"`
	LogToFile   bool   `yaml:"log_to_file"`
	LogFilePath string `yaml:"log_file_path"`

	// gRPC server configuration
	GRPC struct {
		Port int `yaml:"port"`
		TLS  struct {
			CertPath   string `yaml:"cert_path"`
			KeyPath    string `yaml:"key_path"`
			CAPath     string `yaml:"ca_path"`
			ClientAuth string `yaml:"client_auth"` // "none", "request", "require", "require_and_verify"
		} `yaml:"tls"`
	} `yaml:"grpc"`

	// MongoDB configuration
	Mongo struct {
		URI      string       `yaml:"uri"`
		Database string       `yaml:"database"`
		User     string       `yaml:"user"`
		Password SecretString `yaml:"password"`
		// AuthSource is the MongoDB database used to authenticate, e.g. "admin".
		// Equivalent to the ?authSource=<db> URI query param. Only used when
		// User/Password are set via config (not embedded in the URI).
		AuthSource string `yaml:"auth_source"`
		// AuthMechanism overrides the default negotiated mechanism, e.g.
		// "SCRAM-SHA-256", "SCRAM-SHA-1", "MONGODB-X509". Leave empty to let
		// the driver negotiate automatically.
		AuthMechanism string `yaml:"auth_mechanism"`
	} `yaml:"mongo"`

	// Mailbox configuration
	Mailbox struct {
		DefaultRetentionSeconds int `yaml:"default_retention_seconds"`
		MaxBytesPerMailbox      int `yaml:"max_bytes_per_mailbox"`
		MaxEnvelopesPerMailbox  int `yaml:"max_envelopes_per_mailbox"`
	} `yaml:"mailbox"`

	// Session management
	Session struct {
		HeartbeatIntervalSeconds   int `yaml:"heartbeat_interval_seconds"`
		HeartbeatTimeoutMultiplier int `yaml:"heartbeat_timeout_multiplier"`
		ResumeTokenTTLSeconds      int `yaml:"resume_token_ttl_seconds"`
		MaxSessionsPerServerID     int `yaml:"max_sessions_per_server_id"`
	} `yaml:"session"`

	// Flow control settings
	FlowControl struct {
		MaxInflightTotal     int `yaml:"max_inflight_total"`
		MaxInflightCommand   int `yaml:"max_inflight_command"`
		MaxInflightControl   int `yaml:"max_inflight_control"`
		MaxInflightTelemetry int `yaml:"max_inflight_telemetry"`
		MaxBatchBytes        int `yaml:"max_batch_bytes"`
	} `yaml:"flow_control"`

	// Worker pool configuration
	Workers struct {
		SessionPoolSize  int `yaml:"session_pool_size"`
		DeliveryPoolSize int `yaml:"delivery_pool_size"`
		AckPoolSize      int `yaml:"ack_pool_size"`
		MailboxPoolSize  int `yaml:"mailbox_pool_size"`
		TaskQueueSize    int `yaml:"task_queue_size"` // Per-pool channel buffer
	} `yaml:"workers"`

	// Prometheus metrics configuration
	Metrics struct {
		Enabled int `yaml:"enabled"` // 1 to enable, 0 to disable
		Port    int `yaml:"port"`    // HTTP port for /metrics endpoint
	} `yaml:"metrics"`

	// OAuth 2.0 authentication (for validating JWT tokens from clients)
	Auth struct {
		AuthDomain   string `yaml:"auth_domain"`
		AuthAudience string `yaml:"auth_audience"`
	} `yaml:"auth"`
}

var Defaults = map[string]interface{}{
	"LogLevel":    "info",
	"LogFormat":   "json",
	"LogToStderr": true,
	"LogToFile":   false,
	"LogFilePath": "./logs/spine.log",
}

func LoadSettings() *Settings {
	slog.Info("Loading settings")
	// initialize settings with defaults
	settings := Settings{}
	val := reflect.ValueOf(&settings).Elem()
	for key, value := range Defaults {
		field := val.FieldByName(key)
		if field.IsValid() && field.CanSet() {
			field.Set(reflect.ValueOf(value))
		}
	}

	// read config.yaml, config file path set by CONFIG_PATH env var, default to "./config.yaml"
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "./config.yaml"
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		directory, dirErr := os.Getwd()
		if dirErr != nil {
			directory = "unknown"
		}
		dirContents, _ := os.ReadDir(directory)
		slog.Error(
			"failed to read config file",
			"error", err,
			"path", configPath,
			"directory", directory,
			"contents", dirContents,
		)
		panic(err)
	}

	// load yaml
	var fileSettings Settings
	err = yaml.Unmarshal(data, &fileSettings)
	if err != nil {
		panic(err)
	}

	// override defaults with file settings
	fileVal := reflect.ValueOf(&fileSettings).Elem()
	for i := 0; i < fileVal.NumField(); i++ {
		field := fileVal.Type().Field(i)
		fileFieldValue := fileVal.Field(i)
		if fileFieldValue.IsValid() && !fileFieldValue.IsZero() {
			settingsField := val.FieldByName(field.Name)
			if settingsField.IsValid() && settingsField.CanSet() {
				settingsField.Set(fileFieldValue)
			}
		}
	}

	// validate settings
	if err := settings.Validate(); err != nil {
		panic(err)
	}

	return &settings
}

func (s *Settings) Validate() error {
	if !s.LogToStderr && !s.LogToFile {
		return fmt.Errorf("at least one of LogToStderr or LogToFile must be true")
	}
	if s.GRPC.Port <= 0 || s.GRPC.Port > 65535 {
		return fmt.Errorf("grpc.port must be between 1 and 65535")
	}
	if s.Mongo.URI == "" {
		return fmt.Errorf("mongo.uri is required")
	}
	if s.Mongo.Database == "" {
		return fmt.Errorf("mongo.database is required")
	}
	if s.Workers.SessionPoolSize <= 0 {
		return fmt.Errorf("workers.session_pool_size must be positive")
	}
	if s.Workers.DeliveryPoolSize <= 0 {
		return fmt.Errorf("workers.delivery_pool_size must be positive")
	}
	if s.Workers.AckPoolSize <= 0 {
		return fmt.Errorf("workers.ack_pool_size must be positive")
	}
	if s.Workers.MailboxPoolSize <= 0 {
		return fmt.Errorf("workers.mailbox_pool_size must be positive")
	}
	if s.Workers.TaskQueueSize <= 0 {
		return fmt.Errorf("workers.task_queue_size must be positive")
	}
	return nil
}
