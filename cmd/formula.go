/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	_ "embed"
	"fmt"
	"log"
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
		allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Flag("headless", false))...)
		defer cancel()

		ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithDebugf(log.Printf))
		defer cancel()

		templates := conf.Formula.Templates
		m := make(map[string]report, len(templates))
		chromedp.ListenTarget(ctx, func(v interface{}) {
			switch ev := v.(type) {
			case *browser.EventDownloadWillBegin:
				log.Println("EventDownloadWillBegin: ", ev.SuggestedFilename)
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

		daysBefore, err := cmd.Flags().GetUint("days-before")
		if err != nil {
			log.Fatal(err)
		}

		tasks, m := genReport(ctx, daysBefore)
		if err := chromedp.Run(ctx, tasks); err != nil {
			log.Fatal(err)
		}

		home, err := homedir.Dir()
		if err != nil {
			log.Fatal(err)
		}

		targetFiles := make([]string, 0, len(m))
		for _, r := range m {
			if err := os.Rename(filepath.Join(home, "Downloads", r.SourceFile), filepath.Join(conf.OutDir, r.TargetFile)); err != nil {
				log.Fatal(err)
			}
			targetFiles = append(targetFiles, filepath.Join(conf.OutDir, r.TargetFile))
		}

		if err := importData(targetFiles); err != nil {
			log.Fatal(err)
		}

		sendMail, err := cmd.Flags().GetBool("send-mail")
		if err != nil {
			log.Fatal(err)
		}
		if sendMail {
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
		}
	},
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
	tmp, err := os.CreateTemp("", "refresh_all*.ps1")
	if err != nil {
		return errors.Wrap(err, "failed to create temp file")
	}
	defer os.Remove(tmp.Name())

	f, err := os.OpenFile(tmp.Name(), os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return errors.Wrapf(err, "failed to open file %s", tmp.Name())
	}

	_, err = f.Write(refreshAll)
	if err != nil {
		return errors.Wrapf(err, "failed to write to the file %s", tmp.Name())
	}

	if err = f.Close(); err != nil {
		log.Fatal(err)
	}

	args := []string{"-ExecutionPolicy", "Bypass", "-File", tmp.Name()}
	args = append(args, targetFiles...)
	args = append(args, filepath.Join(conf.OutDir, conf.Formula.File))
	winCmd := exec.Command(`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`, args...)
	output, err := winCmd.CombinedOutput()
	log.Printf("output: %s", string(output))
	if err != nil {
		return errors.Wrapf(err, "failed to run the command")
	}
	return nil
}

func waitForInternet(timeout time.Duration) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	to := time.NewTimer(timeout)
	defer to.Stop()
	for {
		select {
		case <-to.C:
			return
		case <-ticker.C:
			_, err := net.Dial("tcp", net.JoinHostPort(conf.SMTP.Host, strconv.Itoa(conf.SMTP.Port)))
			if err == nil {
				return
			}
		}
	}
}
