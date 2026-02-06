package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "alif",
	Short: "Alif CLI - A Zephyr-like tool for Alif Semiconductor boards",
	Long: `Alif CLI helps you build, sign, and flash applications for Alif AK-E7-AIML 
and other Alif boards with ease. It manages toolchains and signing keys 
to simplify your workflow.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
}

func initConfig() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	viper.AddConfigPath(home + "/.alif")
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	viper.ReadInConfig()
}
