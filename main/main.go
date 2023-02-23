package main

import (
	"encoding/json"
	"fmt"
	"github.com/fatih/color"
	"github.com/nwtgck/piping-server-check/check"
	"github.com/spf13/cobra"
	"os"
)

var flag struct {
	serverCommand string
	tlsSkipVerify bool
}

func init() {
	cobra.OnInitialize()
	rootCmd.PersistentFlags().StringVarP(&flag.serverCommand, "server-command", "", "", "Command to run a Piping Server")
	rootCmd.MarkPersistentFlagRequired("server-command")
	rootCmd.PersistentFlags().BoolVarP(&flag.tlsSkipVerify, "tls-skip-verify", "", false, "Skip verify TLS cert (like curl --insecure option)")
}

var rootCmd = &cobra.Command{
	Use:   os.Args[0],
	Short: "Check Piping Server",
	RunE: func(_ *cobra.Command, args []string) error {
		checks := check.AllChecks()
		config := check.Config{
			// TODO: sh -c
			RunServerCmd: []string{"sh", "-c", flag.serverCommand},
		}

		// TODO: hard code
		protocols := []string{check.Http1_1, check.Http1_1_tls}
		for _, c := range checks {
			for _, protocol := range protocols {
				subConfig := check.SubConfig{
					Protocol:          protocol,
					TlsSkipVerifyCert: flag.tlsSkipVerify,
				}
				result := check.RunCheck(&c, &config, &subConfig)
				jsonBytes, err := json.Marshal(&result)
				if err != nil {
					return err
				}

				line := string(jsonBytes)
				if len(result.Errors) == 0 {
					line = color.GreenString(fmt.Sprintf("✔︎%s", line))
				} else {
					line = color.RedString(fmt.Sprintf("✖︎%s", line))
					//line = color.YellowString(fmt.Sprintf("⚠︎%s", line))
				}
				fmt.Println(line)
			}
		}
		// TODO: non-zero exit code when checks have errors
		return nil
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, err.Error())
		os.Exit(-1)
	}
}
