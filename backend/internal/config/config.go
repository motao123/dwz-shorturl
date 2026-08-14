package config

import (
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	PublicDB  DatabaseConfig  `mapstructure:"public_db"`
	Redis     RedisConfig     `mapstructure:"redis"`
	JWT       JWTConfig       `mapstructure:"jwt"`
	Public    PublicConfig    `mapstructure:"public"`
	CORS      CORSConfig      `mapstructure:"cors"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
	Log       LogConfig       `mapstructure:"log"`
	SMTP      SMTPConfig      `mapstructure:"smtp"`
	GeoIP     GeoIPConfig     `mapstructure:"geoip"`
}

// GeoIPConfig locates the ip2region v1 database used to resolve click source
// countries. When DBPath is empty, country resolution is disabled.
type GeoIPConfig struct {
	DBPath string `mapstructure:"db_path"`
}

// SMTPConfig holds outgoing mail settings for expiry reminders.
type SMTPConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	From     string `mapstructure:"from"`
	FromName string `mapstructure:"from_name"`
	SSL      bool   `mapstructure:"ssl"`
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
	Secret          string        `mapstructure:"secret"`
	PreviousSecrets []string      `mapstructure:"previous_secrets"`
	MemberSecret    string        `mapstructure:"member_secret"`
	AccessExpiry    time.Duration `mapstructure:"access_expiry"`
	RefreshExpiry   time.Duration `mapstructure:"refresh_expiry"`
}

type PublicConfig struct {
	BaseURL        string   `mapstructure:"base_url"`
	TrustedProxies []string `mapstructure:"trusted_proxies"`
}

// CORSConfig lists origins allowed to make credentialed cross-origin requests.
// If empty, all cross-origin requests are refused (same-origin only).
type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
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
		v.SetDefault("public_db.host", "127.0.0.1")
		v.SetDefault("public_db.port", 3306)
		v.SetDefault("public_db.user", "")
		v.SetDefault("public_db.password", "")
		v.SetDefault("public_db.dbname", "")
		v.SetDefault("public_db.charset", "utf8mb4")
		v.SetDefault("redis.addr", "127.0.0.1:6379")
		v.SetDefault("redis.password", "")
		v.SetDefault("redis.db", 0)
		v.SetDefault("jwt.secret", "change-me-to-random-32-bytes!!")
		v.SetDefault("jwt.previous_secrets", []string{})
		v.SetDefault("jwt.member_secret", "")
		v.SetDefault("jwt.access_expiry", "2h")
		v.SetDefault("jwt.refresh_expiry", "168h")
		v.SetDefault("public.base_url", "https://1.xk7.cn")
		v.SetDefault("cors.allowed_origins", []string{})
		v.SetDefault("log.level", "info")
		v.SetDefault("log.file", "")

		v.AutomaticEnv()
	v.SetEnvPrefix("DWZ")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

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
