package config

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type Config struct {
	AlifToolsPath  string `mapstructure:"alif_tools_path"`
	CmsisToolbox   string `mapstructure:"cmsis_toolbox_path"`
	GccToolchain   string `mapstructure:"gcc_toolchain_path"`
	CmsisPackRoot  string `mapstructure:"cmsis_pack_root"`
	SigningKeyPath string `mapstructure:"signing_key_path"`
}

func LoadConfig() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	viper.AddConfigPath(filepath.Join(home, ".alif"))
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	err = viper.Unmarshal(&cfg)
	return &cfg, err
}

func SaveConfig(cfg *Config) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	configDir := filepath.Join(home, ".alif")
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		os.MkdirAll(configDir, 0755)
	}

	viper.Set("alif_tools_path", cfg.AlifToolsPath)
	viper.Set("cmsis_toolbox_path", cfg.CmsisToolbox)
	viper.Set("gcc_toolchain_path", cfg.GccToolchain)
	viper.Set("cmsis_pack_root", cfg.CmsisPackRoot)
	viper.Set("signing_key_path", cfg.SigningKeyPath)

	return viper.WriteConfigAs(filepath.Join(configDir, "config.yaml"))
}
