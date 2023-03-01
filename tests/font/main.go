//go:build gofuzz
// +build gofuzz

package fuzz

// Fuzz is a fuzz test.
func Fuzz(data []byte) int {
	ff := canvas.NewFontFamily("")
	_ = ff.LoadFont(data, canvas.FontRegular)
	return 1
}
