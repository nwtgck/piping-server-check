package main

import (
	"encoding/json"
	"fmt"
	"github.com/fatih/color"
	"github.com/nwtgck/piping-server-check/check"
)

func run() error {
	checks := check.AllChecks()
	config := check.Config{
		// TODO:
		RunServerCmd: []string{"sh", "-c", "piping-server --http-port=$HTTP_PORT"},
	}
	for _, c := range checks {
		subConfig := check.SubConfig{
			// TODO:
			Protocol: check.Http1_1,
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
	return nil
}

func main() {
	err := run()
	if err != nil {
		panic(err)
	}
}
