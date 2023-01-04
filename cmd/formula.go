/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/chromedp"
	"github.com/mitchellh/go-homedir"
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

		tasks, m := genReport(ctx)
		if err := chromedp.Run(ctx, tasks); err != nil {
			log.Fatal(err)
		}

		home, err := homedir.Dir()
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("m: %+v", m)
		for _, r := range m {
			if err := os.Rename(filepath.Join(home, "Downloads", r.SourceFile), filepath.Join(conf.OutDir, r.TargetFile)); err != nil {
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
}

func genReport(ctx context.Context) (chromedp.Tasks, map[string]report) {
	selName := `//input[@id="js_usernameid"]`
	selPass := `//input[@id="loginform_password"]`

	tasks, m := genFormulaReports(ctx)
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

func genFormulaReports(ctx context.Context) (chromedp.Tasks, map[string]report) {
	selFormula := `//div[@id="report_group_form"]`
	selDatePicker := `input[name="export_date"]`
	selSubmitForm := `//input[@id="submitform"]`
	selFormDownload := `form[id="formdownload"]`

	yesterday := time.Now().Add(-24 * time.Hour)

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

	var tasks chromedp.Tasks
	for i := range templates {
		tasks = append(tasks, chromedp.Tasks{
			chromedp.Navigate(conf.Formula.URL),
			chromedp.WaitVisible(selFormula),
			chromedp.Click(`//select[@id="save"]`, chromedp.BySearch),
			chromedp.Sleep(1 * time.Second),
			chromedp.SetValue(`//select[@id="save"]`, templates[i].Name, chromedp.BySearch),
			chromedp.WaitVisible(selDatePicker),
			chromedp.SetValue(selDatePicker, yesterday.Format("02/01/2006"), chromedp.ByQuery),
			chromedp.Submit(selSubmitForm),
			chromedp.WaitVisible(selFormDownload),
			browser.SetDownloadBehavior(browser.SetDownloadBehaviorBehaviorDefault).WithEventsEnabled(true),
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