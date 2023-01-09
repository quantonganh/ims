/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"gopkg.in/gomail.v2"
)

// emailCmd represents the email command
var emailCmd = &cobra.Command{
	Use:   "email",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		winCmd := exec.Command("cmd.exe", "/c", "netsh", "wlan", "connect", fmt.Sprintf("ssid=%s", conf.Wifi.SendMail), fmt.Sprintf("name=%s", conf.Wifi.SendMail))
		if err := winCmd.Run(); err != nil {
			log.Fatal(err)
		}
		defer func() {
			winCmd = exec.Command("cmd.exe", "/c", "netsh", "wlan", "connect", fmt.Sprintf("ssid=%s", conf.Wifi.ExportReport), fmt.Sprintf("name=%s", conf.Wifi.ExportReport))
			if err := winCmd.Run(); err != nil {
				log.Fatal(err)
			}
		}()

		waitForInternet(30 * time.Second)

		if err := sendEmail(conf.Formula.Email.Subject, conf.Formula.Email.Body); err != nil {
			log.Fatal(err)
		}
	},
}

func init() {
	formulaCmd.AddCommand(emailCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// emailCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// emailCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

func sendEmail(subject, body string) error {
	m := gomail.NewMessage()
	recipients := make([]string, len(conf.Formula.Email.To))
	for i, r := range conf.Formula.Email.To {
		recipients[i] = m.FormatAddress(r, "")
	}
	m.SetHeader("From", conf.Formula.Email.From)
	m.SetHeader("To", recipients...)
	m.SetHeader("Subject", subject)
	m.Attach(filepath.Join(conf.OutDir, conf.Formula.File))
	m.SetBody("text/html", body)
	d := gomail.NewDialer(conf.SMTP.Host, conf.SMTP.Port, conf.SMTP.Username, conf.SMTP.Password)
	if err := d.DialAndSend(m); err != nil {
		return errors.Errorf("failed to send mail to %s: %v", fmt.Sprintf("%+v\n", conf.Formula.Email.To), err)
	}

	return nil
}
