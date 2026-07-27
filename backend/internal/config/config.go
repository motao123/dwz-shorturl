package config

import (
	"sync"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Redis     RedisConfig     `mapstructure:"redis"`
	JWT       JWTConfig       `mapstructure:"jwt"`
	Public    PublicConfig    `mapstructure:"public"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
	Log       LogConfig       `mapstructure:"log"`
}

type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"`
}

type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	Charset  string `mapstructure:"charset"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type JWTConfig struct {
	Secret        string        `mapstructure:"secret"`
	AccessExpiry  time.Duration `mapstructure:"access_expiry"`
	RefreshExpiry time.Duration `mapstructure:"refresh_expiry"`
}

type PublicConfig struct {
	BaseURL        string   `mapstructure:"base_url"`
	TrustedProxies []string `mapstructure:"trusted_proxies"`
}

type RateLimitConfig struct {
	SingleMax    int `mapstructure:"single_max"`
	SingleWindow int `mapstructure:"single_window"`
	BatchMax     int `mapstructure:"batch_max"`
	BatchWindow  int `mapstructure:"batch_window"`
}

type LogConfig struct {
	Level string `mapstructure:"level"`
	File  string `mapstructure:"file"`
}

var (
	instance *Config
	once     sync.Once
)

func Init(path string) error {
	var err error
	once.Do(func() {
		v := viper.New()
		v.SetConfigFile(path)
		v.SetConfigType("yaml")

		v.SetDefault("server.port", 8080)
		v.SetDefault("server.mode", "release")
		v.SetDefault("database.host", "127.0.0.1")
		v.SetDefault("database.port", 3306)
		v.SetDefault("database.user", "root")
		v.SetDefault("database.password", "")
		v.SetDefault("database.dbname", "dwz_admin")
		v.SetDefault("database.charset", "utf8mb4")
		v.SetDefault("redis.addr", "127.0.0.1:6379")
		v.SetDefault("redis.password", "")
		v.SetDefault("redis.db", 0)
		v.SetDefault("jwt.secret", "change-me-to-random-32-bytes!!")
		v.SetDefault("jwt.access_expiry", "2h")
		v.SetDefault("jwt.refresh_expiry", "168h")
		v.SetDefault("public.base_url", "https://1.xk7.cn")
		v.SetDefault("log.level", "info")
		v.SetDefault("log.file", "")

		v.AutomaticEnv()

		if err = v.ReadInConfig(); err != nil {
			return
		}

		instance = &Config{}
		err = v.Unmarshal(instance)
	})
	return err
}

func Get() *Config {
	return instance
}
