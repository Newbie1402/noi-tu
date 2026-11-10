package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL        string
	Address            string
	APIKey             string
	RequireAPIKey      bool
	AllowedOrigins     map[string]bool
	CollectorInterval  time.Duration
	CollectorAutostart bool
	CollectorOnce      bool
	CollectorBrowser   bool
	StaticToken        string
}

func Load(dotenvPath string) (Config, error) {
	LoadDotEnv(dotenvPath)
	address := os.Getenv("ADDR")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	apiKey := value("API_KEY", "API_KEY_FILE")
	if publicBind(address) && apiKey == "" {
		return Config{}, fmt.Errorf("API_KEY or API_KEY_FILE is required when ADDR is publicly bound")
	}
	interval, err := collectorInterval()
	if err != nil {
		return Config{}, err
	}
	origins := parseOrigins(os.Getenv("ALLOWED_ORIGINS"))
	if len(origins) == 0 {
		origins["https://www.noitu.fun"] = true
	}
	once := os.Getenv("COLLECT_ONCE") == "1"
	if once {
		interval = 0
	}
	return Config{
		DatabaseURL:        value("DATABASE_URL", "DATABASE_URL_FILE"),
		Address:            address,
		APIKey:             apiKey,
		RequireAPIKey:      apiKey != "" || os.Getenv("REQUIRE_API_KEY") == "1",
		AllowedOrigins:     origins,
		CollectorInterval:  interval,
		CollectorAutostart: os.Getenv("COLLECT_AUTOSTART") == "1",
		CollectorOnce:      once,
		CollectorBrowser:   os.Getenv("COLLECT_BROWSER") == "1",
		StaticToken:        os.Getenv("NOITU_ACCESS_TOKEN"),
	}, nil
}

func LoadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, raw, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(strings.TrimPrefix(key, "export "))
		raw = strings.TrimSpace(raw)
		if len(raw) >= 2 && ((raw[0] == '\'' && raw[len(raw)-1] == '\'') || (raw[0] == '"' && raw[len(raw)-1] == '"')) {
			raw = raw[1 : len(raw)-1]
		}
		if key != "" {
			if _, exists := os.LookupEnv(key); !exists {
				_ = os.Setenv(key, raw)
			}
		}
	}
}

func value(name, fileName string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	if path := strings.TrimSpace(os.Getenv(fileName)); path != "" {
		if data, err := os.ReadFile(path); err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	return ""
}

func collectorInterval() (time.Duration, error) {
	value := os.Getenv("COLLECT_INTERVAL_MINUTES")
	if value == "" {
		return 30 * time.Minute, nil
	}
	minutes, err := strconv.Atoi(value)
	if err != nil || minutes < 30 || minutes > 10080 {
		return 0, fmt.Errorf("COLLECT_INTERVAL_MINUTES must be between 30 and 10080")
	}
	return time.Duration(minutes) * time.Minute, nil
}

func parseOrigins(value string) map[string]bool {
	result := map[string]bool{}
	for _, origin := range strings.Split(value, ",") {
		if origin = strings.TrimSpace(origin); origin != "" {
			result[origin] = true
		}
	}
	return result
}

func publicBind(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return true
	}
	return host == "" || host == "0.0.0.0" || host == "::"
}
