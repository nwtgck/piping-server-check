package check

func AllChecks() []Check {
	return []Check{
		post_first(),
		get_first(),
		put(),
		post_cancel_post(),
		service_worker_registration_rejection(),
		post_first_byte_by_byte_streaming(),
		multipart_form_data(),

		// long check
		post_first_chunked_long_transfer(),
	}
}
