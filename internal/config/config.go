package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	// 飞书配置
	FeishuAppID     string
	FeishuAppSecret string

	// Moltbot 配置
	MoltbotConfigPath string
	MoltbotAgentID    string
	GatewayPort       int
	GatewayToken      string

	// 行为配置
	ThinkingThresholdMs int
}

type MoltbotConfig struct {
	Gateway struct {
		Port int    `json:"port"`
		Auth string `json:"auth"`
	} `json:"gateway"`
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[1:])
	}
	return path
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvIntOrDefault(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		var i int
		if _, err := fmt.Sscanf(val, "%d", &i); err == nil {
			return i
		}
	}
	return defaultVal
}

// Flags 存储命令行参数
type Flags struct {
	FeishuAppID      string
	FeishuAppSecret  string
	FeishuSecretPath string
	MoltbotConfig    string
	AgentID          string
	GatewayPort      int
	GatewayToken     string
	ThinkingMs       int
	Version          bool
}

// RegisterFlags 注册命令行参数
func RegisterFlags() *Flags {
	f := &Flags{}
	flag.StringVar(&f.FeishuAppID, "feishu-app-id", "", "飞书应用 App ID")
	flag.StringVar(&f.FeishuAppSecret, "feishu-app-secret", "", "飞书应用 App Secret")
	flag.StringVar(&f.FeishuSecretPath, "feishu-secret-path", "", "飞书应用 Secret 文件路径")
	flag.StringVar(&f.MoltbotConfig, "moltbot-config", "", "Moltbot 配置文件路径")
	flag.StringVar(&f.AgentID, "agent-id", "", "Moltbot Agent ID")
	flag.IntVar(&f.GatewayPort, "gateway-port", 0, "Gateway 端口")
	flag.StringVar(&f.GatewayToken, "gateway-token", "", "Gateway 认证 Token")
	flag.IntVar(&f.ThinkingMs, "thinking-ms", 0, "'正在思考...' 提示延迟毫秒数")
	flag.BoolVar(&f.Version, "version", false, "显示版本号")
	return f
}

// Load 加载配置
func Load(f *Flags) (*Config, error) {
	cfg := &Config{}

	// 优先级: 命令行参数 > 环境变量 > 默认值

	// 飞书 App ID
	cfg.FeishuAppID = f.FeishuAppID
	if cfg.FeishuAppID == "" {
		cfg.FeishuAppID = os.Getenv("FEISHU_APP_ID")
	}
	if cfg.FeishuAppID == "" {
		return nil, fmt.Errorf("飞书 App ID 未配置，请设置 --feishu-app-id 或 FEISHU_APP_ID 环境变量")
	}

	// 飞书 App Secret
	cfg.FeishuAppSecret = f.FeishuAppSecret
	if cfg.FeishuAppSecret == "" {
		cfg.FeishuAppSecret = os.Getenv("FEISHU_APP_SECRET")
	}
	if cfg.FeishuAppSecret == "" {
		// 从文件读取
		secretPath := f.FeishuSecretPath
		if secretPath == "" {
			secretPath = getEnvOrDefault("FEISHU_APP_SECRET_PATH", "~/.moltbot/secrets/feishu_app_secret")
		}
		secretPath = expandPath(secretPath)
		data, err := os.ReadFile(secretPath)
		if err == nil {
			cfg.FeishuAppSecret = strings.TrimSpace(string(data))
		}
	}
	if cfg.FeishuAppSecret == "" {
		return nil, fmt.Errorf("飞书 App Secret 未配置，请设置 --feishu-app-secret、FEISHU_APP_SECRET 或 FEISHU_APP_SECRET_PATH")
	}

	// Moltbot 配置路径
	cfg.MoltbotConfigPath = f.MoltbotConfig
	if cfg.MoltbotConfigPath == "" {
		cfg.MoltbotConfigPath = getEnvOrDefault("MOLTBOT_CONFIG_PATH", "~/.moltbot/moltbot.json")
	}
	cfg.MoltbotConfigPath = expandPath(cfg.MoltbotConfigPath)

	// Agent ID
	cfg.MoltbotAgentID = f.AgentID
	if cfg.MoltbotAgentID == "" {
		cfg.MoltbotAgentID = getEnvOrDefault("MOLTBOT_AGENT_ID", "main")
	}

	// Gateway 配置 - 从 moltbot.json 读取
	cfg.GatewayPort = f.GatewayPort
	cfg.GatewayToken = f.GatewayToken

	if cfg.GatewayPort == 0 || cfg.GatewayToken == "" {
		data, err := os.ReadFile(cfg.MoltbotConfigPath)
		if err == nil {
			var mc MoltbotConfig
			if json.Unmarshal(data, &mc) == nil {
				if cfg.GatewayPort == 0 {
					cfg.GatewayPort = mc.Gateway.Port
				}
				if cfg.GatewayToken == "" {
					cfg.GatewayToken = mc.Gateway.Auth
				}
			}
		}
	}

	// 环境变量覆盖
	if cfg.GatewayPort == 0 {
		cfg.GatewayPort = getEnvIntOrDefault("MOLTBOT_GATEWAY_PORT", 18789)
	}
	if cfg.GatewayToken == "" {
		cfg.GatewayToken = os.Getenv("MOLTBOT_GATEWAY_TOKEN")
	}

	if cfg.GatewayToken == "" {
		return nil, fmt.Errorf("Gateway Token 未配置，请设置 --gateway-token、MOLTBOT_GATEWAY_TOKEN 或在 moltbot.json 中配置")
	}

	// 思考延迟
	cfg.ThinkingThresholdMs = f.ThinkingMs
	if cfg.ThinkingThresholdMs == 0 {
		cfg.ThinkingThresholdMs = getEnvIntOrDefault("FEISHU_THINKING_THRESHOLD_MS", 2500)
	}

	return cfg, nil
}
