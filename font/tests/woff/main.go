//go:build gofuzz
// +build gofuzz

package fuzz

import "github.com/LaminoidStudio/Canvas/font"

// Fuzz is a fuzz test.
func Fuzz(data []byte) int {
	_, _ = font.ParseWOFF(data)
	return 1
}
