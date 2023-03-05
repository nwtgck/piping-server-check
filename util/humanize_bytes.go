package util

import (
	"fmt"
	"math"
)

// (base: https://github.com/schollz/progressbar/blob/9c6973820b2153b15d2e6a08d8705ec981fda59f/progressbar.go#L784-L799)
func HumanizeBytes(s float64) string {
	if math.IsNaN(s) {
		return "NaN"
	}
	sizes := []string{" B", " kB", " MB", " GB", " TB", " PB", " EB"}
	base := 1024.0
	if s < 10 {
		return fmt.Sprintf("%2.0fB", s)
	}
	e := math.Floor(logn(s, base))
	suffix := sizes[int(e)]
	val := math.Floor(s/math.Pow(base, e)*10+0.5) / 10
	f := "%.0f"
	if val < 10 {
		f = "%.1f"
	}

	return fmt.Sprintf(f, val) + suffix
}

// (from: https://github.com/schollz/progressbar/blob/9c6973820b2153b15d2e6a08d8705ec981fda59f/progressbar.go#L784-L799)
func logn(n, b float64) float64 {
	return math.Log(n) / math.Log(b)
}
