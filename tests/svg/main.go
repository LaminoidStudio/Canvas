//go:build gofuzz
// +build gofuzz

package fuzz

// Fuzz is a fuzz test.
func Fuzz(data []byte) int {
	_, _ = canvas.ParseSVG(string(data))
	return 1
}
