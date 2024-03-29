/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/mitchellh/go-homedir"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string
	conf    *config
	log     zerolog.Logger
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "ims",
	Short: "A brief description of your application",
	Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		f, err := os.OpenFile(
			fmt.Sprintf("ims_%s.txt", time.Now().Format("20060102150405")),
			os.O_APPEND|os.O_CREATE|os.O_WRONLY,
			0664,
		)
		if err != nil {
			return err
		}

		ctx := context.WithValue(cmd.Context(), "f", f)
		cmd.SetContext(ctx)

		mw := io.MultiWriter(os.Stdout, f)
		log = zerolog.New(mw).With().Timestamp().Logger()
		return nil
	},
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
	PostRun: func(cmd *cobra.Command, args []string) {
		v := cmd.Context().Value("f")
		f, ok := v.(*os.File)
		if ok {
			if err := f.Close(); err != nil {
				log.Err(err).Msg("failed to close file")
			}
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

type config struct {
	ProfileDir string  `yaml:"profileDir"`
	URL        string  `yaml:"url"`
	Username   string  `yaml:"username"`
	Password   string  `yaml:"password"`
	Formula    formula `yaml:"formula"`
	OutDir     string  `yaml:"outDir"`
	ExcelPath  string  `yaml:"excelPath"`
	Wifi       Wifi    `yaml:"wifi"`
	SMTP       SMTP    `yaml:"smtp"`
}

type formula struct {
	URL       string     `yaml:"url"`
	Templates []template `yaml:"templates"`
	File      string     `yaml:"file"`
	Email     email      `yaml:"email"`
}

type template struct {
	Name       string `yaml:"name"`
	TargetFile string `yaml:"targetFile"`
}

type email struct {
	From    string   `yaml:"from"`
	To      []string `yaml:"to"`
	Subject string   `yaml:"subject"`
	Body    string   `yaml:"body"`
}

type Wifi struct {
	ExportReport string `yaml:"exportReport"`
	SendMail     string `yaml:"sendMail"`
}

type SMTP struct {
	Host     string
	Port     int
	Username string
	Password string
}

func init() {
	cobra.OnInitialize(initConfig)
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.ims.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.ims.yaml)")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".ims" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".ims")
	}

	viper.SetEnvPrefix("ims")
	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err != nil {
		log.Err(err).Msg("failed to load the config file")
		return
	}

	if err := viper.Unmarshal(&conf); err != nil {
		log.Err(err).Msg("failed to unmarshal the config into a struct")
		return
	}
}
