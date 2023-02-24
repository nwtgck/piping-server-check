package main

import (
	"encoding/json"
	"fmt"
	"github.com/fatih/color"
	"github.com/nwtgck/piping-server-check/check"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"
	"net/url"
	"os"
	"strings"
	"time"
)

var flag struct {
	serverCommand         string
	serverSchemalessUrl   string
	tlsSkipVerify         bool
	http1_1               bool
	http1_1Tls            bool
	h2                    bool
	h2c                   bool
	h3                    bool
	compromiseResultNames []string
}

func init() {
	cobra.OnInitialize()
	rootCmd.PersistentFlags().StringVarP(&flag.serverCommand, "server-command", "", "", "Command to run a Piping Server")
	rootCmd.PersistentFlags().StringVarP(&flag.serverSchemalessUrl, "server-schemaless-url", "", "", "Piping Server schemaless URL (e.g. //ppng.io/myspace)")
	rootCmd.PersistentFlags().BoolVarP(&flag.tlsSkipVerify, "tls-skip-verify", "", false, "Skip verify TLS cert (like curl --insecure option)")
	rootCmd.PersistentFlags().BoolVarP(&flag.http1_1, "http1.1", "", false, "HTTP/1.1 cleartext")
	rootCmd.PersistentFlags().BoolVarP(&flag.http1_1Tls, "http1.1-tls", "", false, "HTTP/1.1 over TLS")
	rootCmd.PersistentFlags().BoolVarP(&flag.h2, "h2", "", false, "HTTP/2 (TLS)")
	rootCmd.PersistentFlags().BoolVarP(&flag.h2c, "h2c", "", false, "HTTP/2 cleartext")
	rootCmd.PersistentFlags().BoolVarP(&flag.h3, "h3", "", false, "HTTP/3")
	rootCmd.PersistentFlags().StringArrayVarP(&flag.compromiseResultNames, "compromise", "", nil, "Compromise results which have errors and exit 0 if no other errors exist (e.g. --compromise get_first --compromise put.transferred)")
}

var rootCmd = &cobra.Command{
	Use:   os.Args[0],
	Short: "Check Piping Server",
	RunE: func(_ *cobra.Command, args []string) error {
		var config check.Config
		if flag.serverCommand != "" {
			// TODO: sh -c
			config.RunServerCmd = []string{"sh", "-c", flag.serverCommand}
		} else if flag.serverSchemalessUrl != "" {
			_, err := url.Parse(flag.serverSchemalessUrl)
			if err != nil || !strings.HasPrefix(flag.serverSchemalessUrl, "//") {
				fmt.Fprintf(os.Stderr, "--server-schemaless-url should be like '//ppng.io'\n")
				os.Exit(1)
			}
			config.ServerSchemalessUrl = flag.serverSchemalessUrl
		} else {
			fmt.Fprintf(os.Stderr, "Specify --server-command or --server-schemaless-url\n")
			os.Exit(1)
		}
		checks := check.AllChecks()
		var protocols []check.Protocol
		if flag.http1_1 {
			protocols = append(protocols, check.Http1_1)
		}
		if flag.http1_1Tls {
			protocols = append(protocols, check.Http1_1_tls)
		}
		if flag.h2 {
			protocols = append(protocols, check.H2)
		}
		if flag.h2c {
			protocols = append(protocols, check.H2c)
		}
		if flag.h3 {
			protocols = append(protocols, check.H3)
		}
		if len(protocols) == 0 {
			fmt.Fprintf(os.Stderr, "Specify --http1.1 or --http1.1-tls to check\n")
		}
		config.TlsSkipVerifyCert = flag.tlsSkipVerify
		// TODO: to be option
		config.SenderResponseBeforeReceiverTimeout = 5 * time.Second
		// TODO: to be option
		config.FirstByteCheckTimeout = 5 * time.Second
		// TODO: to be option
		config.GetResponseReceivedTimeout = 5 * time.Second
		config.GetReqWroteRequestWaitForH3 = 3 * time.Second

		shouldExitWithNonZero := false
		for result := range runChecks(checks, &config, protocols) {
			jsonBytes, err := json.Marshal(&result)
			if err != nil {
				return err
			}
			line := string(jsonBytes)
			if len(result.Errors) != 0 {
				if slices.Contains(flag.compromiseResultNames, result.Name) {
					line = color.MagentaString(fmt.Sprintf("✖︎ %s", line))
				} else {
					shouldExitWithNonZero = true
					line = color.RedString(fmt.Sprintf("✖︎ %s", line))
				}
			} else if len(result.Warnings) != 0 {
				line = color.YellowString(fmt.Sprintf("⚠︎ %s", line))
			} else {
				line = color.GreenString(fmt.Sprintf("✔︎ %s", line))
			}
			fmt.Println(line)
		}
		if shouldExitWithNonZero {
			os.Exit(1)
		}
		return nil
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, err.Error())
		os.Exit(-1)
	}
}

func runChecks(checks []check.Check, commonConfig *check.Config, protocols []check.Protocol) <-chan check.Result {
	ch := make(chan check.Result)
	go func() {
		for _, c := range checks {
			for _, protocol := range protocols {
				config := *commonConfig
				config.Protocol = protocol
				// TODO: timeout for RunCheck considering long-time check
				// TODO: Use AcceptedProtocols
				check.RunCheck(&c, &config, ch)
			}
		}
		close(ch)
	}()
	return ch
}
