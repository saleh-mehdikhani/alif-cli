package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

const alifVersion = "0.3.0"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of Alif CLI",
	Long:  `All software has versions. This is Alif CLI's`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Alif CLI v%s\n", alifVersion)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.Version = alifVersion
}
