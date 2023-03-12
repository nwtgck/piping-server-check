package main

import (
	"encoding/json"
	"fmt"
	"github.com/fatih/color"
	"github.com/nwtgck/piping-server-check/check"
	"github.com/nwtgck/piping-server-check/version"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"
)

var flag struct {
	ServerCommand          string          `json:"server_command,omitempty"`
	HealthCheckPath        string          `json:"health_check_path"`
	ServerSchemalessUrl    string          `json:"server_schemaless_url,omitempty"`
	TlsSkipVerify          bool            `json:"tls_skip_verify"`
	Http1_0                bool            `json:"http1.0"`
	Http1_0Tls             bool            `json:"http1.0-tls"`
	Http1_1                bool            `json:"http1.1"`
	Http1_1Tls             bool            `json:"http1.1-tls"`
	H2                     bool            `json:"h2"`
	H2c                    bool            `json:"h2c"`
	H3                     bool            `json:"h3"`
	Compromises            []string        `json:"compromise,omitempty"`
	LongTransferBytePerSec int             `json:"long_transfer_speed_byte,omitempty"`
	TransferSpans          []time.Duration `json:"-"`
	TransferSpansForJson   []jsonDuration  `json:"transfer_spans,omitempty"`
	Concurrency            uint            `json:"concurrency"`
	ResultJSONLPath        string          `json:"result_jsonl_path,omitempty"`
}

type jsonDuration struct {
	time.Duration
}

func (d *jsonDuration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func init() {
	cobra.OnInitialize()
	rootCmd.PersistentFlags().StringVarP(&flag.ServerCommand, "server-command", "", "", "Command to run a Piping Server. Use $HTTP_PORT, $HTTPS_PORT in command")
	rootCmd.PersistentFlags().StringVarP(&flag.HealthCheckPath, "health-check-path", "", "/", "Health check path for server command. (e.g. /, /version)")
	rootCmd.PersistentFlags().StringVarP(&flag.ServerSchemalessUrl, "server-schemaless-url", "", "", "Piping Server schemaless URL (e.g. //ppng.io/myspace)")
	rootCmd.PersistentFlags().BoolVarP(&flag.TlsSkipVerify, "tls-skip-verify", "", false, "Skip verify TLS cert (like curl --insecure option)")
	rootCmd.PersistentFlags().BoolVarP(&flag.Http1_0, "http1.0", "", false, "HTTP/1.0 cleartext")
	rootCmd.PersistentFlags().BoolVarP(&flag.Http1_0Tls, "http1.0-tls", "", false, "HTTP/1.0 over TLS")
	rootCmd.PersistentFlags().BoolVarP(&flag.Http1_1, "http1.1", "", false, "HTTP/1.1 cleartext")
	rootCmd.PersistentFlags().BoolVarP(&flag.Http1_1Tls, "http1.1-tls", "", false, "HTTP/1.1 over TLS")
	rootCmd.PersistentFlags().BoolVarP(&flag.H2, "h2", "", false, "HTTP/2 (TLS)")
	rootCmd.PersistentFlags().BoolVarP(&flag.H2c, "h2c", "", false, "HTTP/2 cleartext")
	rootCmd.PersistentFlags().BoolVarP(&flag.H3, "h3", "", false, "HTTP/3")
	rootCmd.PersistentFlags().StringArrayVarP(&flag.Compromises, "compromise", "", nil, "Compromise results which have errors and exit 0 if no other errors exist (e.g. --compromise get_first --compromise http1.1/put.transferred)")
	rootCmd.PersistentFlags().IntVarP(&flag.LongTransferBytePerSec, "transfer-speed-byte", "", 1024*1024, "transfer byte-per-second used in long transfer checks")
	rootCmd.PersistentFlags().DurationSliceVarP(&flag.TransferSpans, "transfer-span", "", nil, "transfer spans used in long transfer checks (e.g. 3s)")
	rootCmd.PersistentFlags().UintVarP(&flag.Concurrency, "concurrency", "", 1, "1 means running check one by one. 2 means that two checks run concurrently")
	rootCmd.PersistentFlags().StringVarP(&flag.ResultJSONLPath, "result-jsonl-path", "", "", "output file path of result JSONL")
}

var rootCmd = &cobra.Command{
	Use:   os.Args[0],
	Short: "Check Piping Server",
	RunE: func(_ *cobra.Command, args []string) error {
		// https://docs.github.com/en/actions/learn-github-actions/variables#default-environment-variables
		// https://github.com/fatih/color/blob/d080a5b7925fbc23275fea62c8f5d82991bfead4/README.md?plain=1#L160
		if os.Getenv("GITHUB_ACTIONS") == "true" {
			color.NoColor = false
		}
		var commonConfig check.Config
		if flag.ServerCommand != "" {
			// TODO: sh -c
			commonConfig.RunServerCmd = []string{"sh", "-c", flag.ServerCommand}
		} else if flag.ServerSchemalessUrl != "" {
			_, err := url.Parse(flag.ServerSchemalessUrl)
			if err != nil || !strings.HasPrefix(flag.ServerSchemalessUrl, "//") {
				fmt.Fprintf(os.Stderr, "--server-schemaless-url should be like '//ppng.io'\n")
				os.Exit(1)
			}
			commonConfig.ServerSchemalessUrl = flag.ServerSchemalessUrl
		} else {
			fmt.Fprintf(os.Stderr, "Specify --server-command or --server-schemaless-url\n")
			os.Exit(1)
		}
		commonConfig.HealthCheckPath = flag.HealthCheckPath
		checks := check.AllChecks()
		var protocols []check.Protocol
		if flag.Http1_0 {
			protocols = append(protocols, check.ProtocolHttp1_0)
		}
		if flag.Http1_0Tls {
			protocols = append(protocols, check.ProtocolHttp1_0_tls)
		}
		if flag.Http1_1 {
			protocols = append(protocols, check.ProtocolHttp1_1)
		}
		if flag.Http1_1Tls {
			protocols = append(protocols, check.ProtocolHttp1_1_tls)
		}
		if flag.H2 {
			protocols = append(protocols, check.ProtocolH2)
		}
		if flag.H2c {
			protocols = append(protocols, check.ProtocolH2c)
		}
		if flag.H3 {
			protocols = append(protocols, check.ProtocolH3)
		}
		if len(protocols) == 0 {
			fmt.Fprintf(os.Stderr, "Specify --http1.1, --http1.1-tls or other protocols to check\n")
		}
		commonConfig.TlsSkipVerifyCert = flag.TlsSkipVerify
		commonConfig.Concurrency = flag.Concurrency
		// TODO: to be option
		commonConfig.SenderResponseBeforeReceiverTimeout = 5 * time.Second
		// TODO: to be option
		commonConfig.FirstByteCheckTimeout = 5 * time.Second
		// TODO: to be option
		commonConfig.GetResponseReceivedTimeout = 5 * time.Second
		commonConfig.GetReqWroteRequestWaitForH3 = 3 * time.Second
		// TODO: to be option
		commonConfig.WaitDurationAfterSenderCancel = 1 * time.Second
		// TODO: to be option
		commonConfig.WaitDurationBetweenReceiverWroteRequestAndCancel = 3 * time.Second
		// TODO: to be option
		commonConfig.WaitDurationAfterReceiverCancel = 3 * time.Second
		// TODO: to be option
		commonConfig.FixedLengthBodyGetTimeout = 6 * time.Second
		commonConfig.TransferBytePerSec = flag.LongTransferBytePerSec
		slices.Sort(flag.TransferSpans)
		commonConfig.SortedTransferSpans = flag.TransferSpans

		shouldExitWithNonZero := false
		var jsonlBytes []byte
		for _, duration := range flag.TransferSpans {
			flag.TransferSpansForJson = append(flag.TransferSpansForJson, jsonDuration{duration})
		}
		header := struct {
			Version string `json:"version"`
			Engine  string `json:"engine"`
			Os      string `json:"os"`
			Arch    string `json:"arch"`
			Options any    `json:"options"`
		}{
			Version: version.Version,
			Engine:  runtime.Version(),
			Os:      runtime.GOOS,
			Arch:    runtime.GOARCH,
			Options: flag,
		}
		jsonBytes, err := json.Marshal(&header)
		if err != nil {
			return err
		}
		jsonlBytes = append(jsonlBytes, append(jsonBytes, 10)...)
		fmt.Println(fmt.Sprintf("　 %s", string(jsonBytes)))
		// TODO: output version
		for result := range check.RunChecks(checks, &commonConfig, protocols) {
			jsonBytes, err := json.Marshal(&result)
			if err != nil {
				return err
			}
			line := string(jsonBytes)
			jsonlBytes = append(jsonlBytes, append(jsonBytes, 10)...)
			if len(result.Errors) != 0 {
				if shouldCompromise(&result) {
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
		if flag.ResultJSONLPath != "" {
			err := os.WriteFile(flag.ResultJSONLPath, jsonlBytes, 0644)
			if err != nil {
				return err
			}
		}
		if shouldExitWithNonZero {
			os.Exit(1)
		}
		return nil
	},
}

func shouldCompromise(result *check.Result) bool {
	for _, compromise := range flag.Compromises {
		splits := strings.SplitN(compromise, "/", 2)
		if len(splits) == 1 && splits[0] == result.Name {
			return true
		}
		if len(splits) == 2 && check.Protocol(splits[0]) == result.Protocol && splits[1] == result.Name {
			return true
		}
	}
	return false
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, err.Error())
		os.Exit(-1)
	}
}
