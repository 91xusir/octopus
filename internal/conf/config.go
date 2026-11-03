package conf

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/spf13/viper"
)

type Server struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

type Log struct {
	Level string `mapstructure:"level"`
}

type Database struct {
	Type string `mapstructure:"type"`
	Path string `mapstructure:"path"`
}

type Config struct {
	Server   Server   `mapstructure:"server"`
	Log      Log      `mapstructure:"log"`
	Database Database `mapstructure:"database"`
}

var AppConfig Config

const configFile = "config.json"

// Load 加载应用配置
func Load(path string) error {
	v := viper.New()

	// 配置默认值
	setDefaults(v)

	// 环境变量
	v.SetEnvPrefix(APP_NAME)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// 配置文件
	if path == "" {
		path = filepath.Join(configBase(), "data", configFile)
	} else if !filepath.IsAbs(path) {
		path = filepath.Join(configBase(), path)
	}

	// SetConfigFile 指定路径后, 文件缺失时 ReadInConfig 返回的是 PathError 而非
	// ConfigFileNotFoundError, 故用 Stat 显式判断存在性: 不存在则生成默认配置文件。
	if _, err := os.Stat(path); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat config: %w", err)
		}

		log.Infof("Config file not found, creating default config: %s", path)

		// 创建配置目录
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create config directory: %w", err)
		}

		v.SetConfigFile(path)
		v.SetConfigType("json")

		// 写入默认配置
		if err := v.SafeWriteConfigAs(path); err != nil {
			return fmt.Errorf("write default config: %w", err)
		}
	} else {
		v.SetConfigFile(path)
		v.SetConfigType("json")

		// 读取配置
		if err := v.ReadInConfig(); err != nil {
			return fmt.Errorf("read config: %w", err)
		}
	}

	// 解析配置
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}

	// 校验配置
	if err := validate(&cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// 处理数据库路径
	if !filepath.IsAbs(cfg.Database.Path) {
		cfg.Database.Path = filepath.Join(configBase(), cfg.Database.Path)
	}

	AppConfig = cfg

	log.Infof("Using config file: %s", v.ConfigFileUsed())

	return nil
}

// setDefaults 设置默认配置
func setDefaults(v *viper.Viper) {
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", PORT)

	v.SetDefault("log.level", "info")

	v.SetDefault("database.type", "sqlite")
	v.SetDefault("database.path", "data/data.db")
}

// validate 校验配置
func validate(cfg *Config) error {
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}

	switch strings.ToLower(cfg.Log.Level) {
	case "debug", "info", "warn", "error", "fatal":
	default:
		return fmt.Errorf("unsupported log.level: %s", cfg.Log.Level)
	}

	switch strings.ToLower(cfg.Database.Type) {
	case "sqlite":
	default:
		return fmt.Errorf("unsupported database.type: %s", cfg.Database.Type)
	}

	if strings.TrimSpace(cfg.Database.Path) == "" {
		return fmt.Errorf("database.path cannot be empty")
	}

	return nil
}

// isGoRun 判断当前进程是否由 go run 启动
func isGoRun() bool {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return false
	}

	return bi.Main.Path == "command-line-arguments"
}

// configBase 返回应用根目录
func configBase() string {
	if isGoRun() {
		return "."
	}

	exe, err := os.Executable()
	if err != nil {
		return "."
	}

	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	return filepath.Dir(exe)
}
