package util

import "net/http"

func IsHttp4xxError(resp *http.Response) bool {
	return 400 <= resp.StatusCode && resp.StatusCode < 500
}
