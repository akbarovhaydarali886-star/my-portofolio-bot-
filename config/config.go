package config

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

type Config struct {
	BotToken      string
	ChannelID     string
	AdminChatID   string
	Port          string
	PortfolioURL  string
	DeveloperName string
	DeveloperTG   string
	mu            sync.Mutex
}

// Load loads configuration from environment variables and an optional .env file.
func Load() *Config {
	loadDotEnv(".env")
	loadDotEnv("../.env")

	cfg := &Config{
		BotToken:      getEnv("TELEGRAM_BOT_TOKEN", ""),
		ChannelID:     getEnv("TELEGRAM_CHANNEL_ID", ""),
		AdminChatID:   getEnv("TELEGRAM_ADMIN_CHAT_ID", ""),
		Port:          getEnv("PORT", "8080"),
		PortfolioURL:  getEnv("PORTFOLIO_URL", "https://haydarali.uz"),
		DeveloperName: getEnv("DEVELOPER_NAME", "Akbarov Haydarali"),
		DeveloperTG:   getEnv("DEVELOPER_TG", "@haydaraliakbarov"),
	}

	return cfg
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return defaultVal
}

// UpdateChannelID updates ChannelID dynamically and saves it to .env
func (c *Config) UpdateChannelID(newChannelID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ChannelID = newChannelID
	_ = os.Setenv("TELEGRAM_CHANNEL_ID", newChannelID)
	updateEnvFileKey("TELEGRAM_CHANNEL_ID", newChannelID)
}

// UpdateAdminChatID updates AdminChatID dynamically and saves it to .env
func (c *Config) UpdateAdminChatID(newAdminID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.AdminChatID = newAdminID
	_ = os.Setenv("TELEGRAM_ADMIN_CHAT_ID", newAdminID)
	updateEnvFileKey("TELEGRAM_ADMIN_CHAT_ID", newAdminID)
}

func updateEnvFileKey(key, value string) {
	filepath := ".env"
	content, err := os.ReadFile(filepath)
	if err != nil {
		// try parent .env
		filepath = "../.env"
		content, err = os.ReadFile(filepath)
		if err != nil {
			return
		}
	}

	lines := strings.Split(string(content), "\n")
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, key+"=") {
			lines[i] = fmt.Sprintf("%s=%s", key, value)
			found = true
			break
		}
	}

	if !found {
		lines = append(lines, fmt.Sprintf("%s=%s", key, value))
	}

	_ = os.WriteFile(filepath, []byte(strings.Join(lines, "\n")), 0644)
}

// Simple .env parser to avoid external dependencies
func loadDotEnv(filepath string) {
	file, err := os.Open(filepath)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			if (strings.HasPrefix(v, "\"") && strings.HasSuffix(v, "\"")) ||
				(strings.HasPrefix(v, "'") && strings.HasSuffix(v, "'")) {
				v = v[1 : len(v)-1]
			}
			if _, exists := os.LookupEnv(k); !exists {
				_ = os.Setenv(k, v)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("Warning reading %s: %v", filepath, err)
	}
}
