package check

func AllChecks() []Check {
	return []Check{
		post_first(),
		get_first(),
		put(),
		post_first_byte_by_byte_streaming(),
	}
}
