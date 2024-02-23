/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	_ "embed"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/chromedp"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/gomail.v2"
)

type report struct {
	SourceFile string
	TargetFile string
}

// formulaCmd represents the report command
var formulaCmd = &cobra.Command{
	Use:   "formula",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		daysBefore, err := cmd.Flags().GetUint("days-before")
		if err != nil {
			log.Err(err).Msg("failed to get the value of days-before")
			return
		}

		sendMail, err := cmd.Flags().GetBool("send-mail")
		if err != nil {
			log.Err(err).Msg("failed to get the value of send-mail")
			return
		}

		if err := run(daysBefore, sendMail); err != nil {
			log.Err(err).Msg("failed to run the command")
		}
	},
}

func run(daysBefore uint, sendMail bool) error {
	opts := append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
	)
	if conf.ProfileDir != "" {
		opts = append(
			opts,
			chromedp.UserDataDir(conf.UserDataDir),
			chromedp.Flag("profile-directory", conf.ProfileDir),
		)
	}
	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithDebugf(log.Printf))
	defer cancel()

	templates := conf.Formula.Templates
	m := make(map[string]report, len(templates))
	chromedp.ListenTarget(ctx, func(v interface{}) {
		switch ev := v.(type) {
		case *browser.EventDownloadWillBegin:
			log.Info().Msgf("EventDownloadWillBegin: %s", ev.SuggestedFilename)
			templateName := strings.Split(ev.SuggestedFilename, "_")[0]
			_, ok := m[templateName]
			if !ok {
				m[templateName] = report{
					SourceFile: ev.SuggestedFilename,
					TargetFile: getTargetFile(templateName),
				}
			}
		default:
			return
		}
	})

	tasks, m := genReport(ctx, daysBefore)
	if err := chromedp.Run(ctx, tasks); err != nil {
		return fmt.Errorf("failed to run chromedp: %w", err)
	}

	home, err := homedir.Dir()
	if err != nil {
		return fmt.Errorf("failed to get home dir: %w", err)
	}

	targetFiles := make([]string, 0, len(m))
	for _, r := range m {
		if err := os.Rename(filepath.Join(home, "Downloads", r.SourceFile), filepath.Join(conf.OutDir, r.TargetFile)); err != nil {
			return fmt.Errorf("failed to rename: %w", err)
		}
		targetFiles = append(targetFiles, filepath.Join(conf.OutDir, r.TargetFile))
	}

	if err := importData(targetFiles); err != nil {
		return err
	}

	if sendMail {
		defer func() {
			winCmd := exec.Command("cmd.exe", "/c", "netsh", "wlan", "connect", fmt.Sprintf("ssid=%s", conf.Wifi.ExportReport), fmt.Sprintf("name=%s", conf.Wifi.ExportReport))
			output, err := winCmd.CombinedOutput()
			if err != nil {
				log.Err(err).Msgf("failed to switch wifi to %s: %s: %w", conf.Wifi.ExportReport, string(output), err)
			}
			log.Printf("%s: %s", conf.Wifi.ExportReport, output)
		}()

		retry(3*time.Second, 30*time.Second, func() bool {
			winCmd := exec.Command("cmd.exe", "/c", "netsh", "wlan", "connect", fmt.Sprintf("ssid=%s", conf.Wifi.SendMail), fmt.Sprintf("name=%s", conf.Wifi.SendMail))
			output, err := winCmd.CombinedOutput()
			if err == nil {
				log.Printf("%s: %s", conf.Wifi.SendMail, output)
				return true
			}
			log.Err(err).Msgf("failed to switch wifi to %s: %s: %w", conf.Wifi.SendMail, string(output), err)
			return false
		})

		retry(time.Second, 30*time.Second, func() bool {
			address := net.JoinHostPort(conf.SMTP.Host, strconv.Itoa(conf.SMTP.Port))
			log.Printf("connecting to the %s", address)
			_, err := net.Dial("tcp", address)
			if err == nil {
				return true
			}
			return false
		})

		if err := sendEmail(conf.Formula.Email.Subject, conf.Formula.Email.Body); err != nil {
			return err
		}
	}

	return nil
}

func init() {
	rootCmd.AddCommand(formulaCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// formulaCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// formulaCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	formulaCmd.Flags().UintP("days-before", "d", 1, "number of days before today")
	formulaCmd.Flags().BoolP("send-mail", "s", false, "send mail after importing data")
}

func genReport(ctx context.Context, daysBefore uint) (chromedp.Tasks, map[string]report) {
	selName := `//input[@id="js_usernameid"]`
	selPass := `//input[@id="loginform_password"]`

	tasks, m := genFormulaReports(ctx, daysBefore)
	return chromedp.Tasks{
		chromedp.Navigate(viper.GetString("url")),
		chromedp.WaitVisible(selPass),
		chromedp.SendKeys(selName, viper.GetString("username")),
		chromedp.SendKeys(selPass, viper.GetString("password")),
		chromedp.Submit(selPass),
		chromedp.WaitVisible(`//div[@id="cssmenu"]`),
		tasks,
	}, m
}

func genFormulaReports(ctx context.Context, daysBefore uint) (chromedp.Tasks, map[string]report) {
	selFormula := `//div[@id="report_group_form"]`
	selDatePicker := `input[name="export_date"]`
	selSubmitForm := `//input[@id="submitform"]`
	selFormDownload := `form[id="formdownload"]`

	exportDate := time.Now().Add(-time.Duration(daysBefore) * 24 * time.Hour)

	templates := conf.Formula.Templates
	m := make(map[string]report, len(templates))

	var tasks chromedp.Tasks
	for i := range templates {
		tasks = append(tasks, chromedp.Tasks{
			chromedp.Navigate(conf.Formula.URL),
			chromedp.WaitVisible(selFormula),
			chromedp.Click(`//select[@id="save"]`, chromedp.BySearch),
			chromedp.Sleep(1 * time.Second),
			chromedp.SetValue(`//select[@id="save"]`, templates[i].Name, chromedp.BySearch),
			chromedp.WaitVisible(selDatePicker),
			chromedp.SetValue(selDatePicker, exportDate.Format("02/01/2006"), chromedp.ByQuery),
			chromedp.Submit(selSubmitForm),
			chromedp.WaitVisible(selFormDownload),
			browser.SetDownloadBehavior(browser.SetDownloadBehaviorBehaviorDefault).
				WithEventsEnabled(true),
			chromedp.Submit(selFormDownload),
			chromedp.Sleep(3 * time.Second),
		})
	}

	return tasks, m
}

func getTargetFile(templateName string) string {
	templates := conf.Formula.Templates
	for i := range templates {
		if templates[i].Name == templateName {
			return templates[i].TargetFile
		}
	}
	return ""
}

//go:embed refresh_all.ps1
var refreshAll []byte

func importData(targetFiles []string) error {
	f, err := os.CreateTemp("", "refresh_all*.ps1")
	if err != nil {
		return errors.Wrap(err, "failed to create temp file")
	}
	defer os.Remove(f.Name())

	_, err = f.Write(refreshAll)
	if err != nil {
		return errors.Wrapf(err, "failed to write to the file %s", f.Name())
	}

	if err := f.Close(); err != nil {
		return errors.Wrapf(err, "failed to close file %s", f.Name())
	}

	args := []string{"-ExecutionPolicy", "Bypass", "-File", f.Name()}
	args = append(args, targetFiles...)
	args = append(args, filepath.Join(conf.OutDir, conf.Formula.File))
	winCmd := exec.Command(`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`, args...)
	output, err := winCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to run the command: %s: %w", string(output), err)
	}
	log.Printf("output: %s", string(output))
	return nil
}

func retry(interval, timeout time.Duration, f func() bool) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	to := time.NewTimer(timeout)
	defer to.Stop()
	for {
		select {
		case <-ticker.C:
			if f() {
				return
			}
		case <-to.C:
			log.Info().Msgf("timed out after %s", timeout.String())
			return
		}
	}
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
