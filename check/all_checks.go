package check

func AllChecks() []Check {
	return []Check{
		post_first(),
		get_first(),
		put(),
		service_worker_registration_rejection(),
		post_first_byte_by_byte_streaming(),
	}
}
