package config

import (
	"fmt"
	"net/url"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	MySQL     MySQLConfig     `mapstructure:"mysql"`
	JWT       JWTConfig       `mapstructure:"jwt"`
	Snowflake SnowflakeConfig `mapstructure:"snowflake"`
	Storage   StorageConfig   `mapstructure:"storage"`
	Log       LogConfig       `mapstructure:"log"`
}

type ServerConfig struct {
	Mode         string        `mapstructure:"mode"`
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
}

type MySQLConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	Username        string        `mapstructure:"username"`
	Password        string        `mapstructure:"password"`
	Database        string        `mapstructure:"database"`
	Charset         string        `mapstructure:"charset"`
	ParseTime       bool          `mapstructure:"parse_time"`
	Loc             string        `mapstructure:"loc"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

// DSN 拼接 GORM 用的连接串
func (m MySQLConfig) DSN() string {
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=%t&loc=%s",
		m.Username, m.Password, m.Host, m.Port, m.Database,
		m.Charset, m.ParseTime, url.QueryEscape(m.Loc),
	)
}

type JWTConfig struct {
	Secret string        `mapstructure:"secret"`
	Issuer string        `mapstructure:"issuer"`
	Expire time.Duration `mapstructure:"expire"`
}

type SnowflakeConfig struct {
	NodeID int64 `mapstructure:"node_id"`
}

type StorageConfig struct {
	BaseDir        string `mapstructure:"base_dir"`
	VideoDir       string `mapstructure:"video_dir"`
	CoverDir       string `mapstructure:"cover_dir"`
	VideoURLPrefix string `mapstructure:"video_url_prefix"`
	CoverURLPrefix string `mapstructure:"cover_url_prefix"`
	MaxVideoSizeMB int    `mapstructure:"max_video_size_mb"`
}

type LogConfig struct {
	Level    string `mapstructure:"level"`
	Encoding string `mapstructure:"encoding"`
}

// Load 从 path（文件夹或具体路径）加载配置；同时支持 XFS_ 前缀环境变量覆盖。
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(path)
	v.AddConfigPath(".")
	v.AddConfigPath("./config")

	v.SetEnvPrefix("XFS")
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &cfg, nil
}
