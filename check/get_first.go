package check

func get_first() Check {
	return Check{
		Name:              checkName(),
		AcceptedProtocols: []string{Http1_0, Http1_1, H2, H2c},
		run: func(config *Config, subConfig *SubConfig) (result Result) {
			result.Errors = append(result.Errors, NewError("not implemented", nil))
			return
		},
	}
}
