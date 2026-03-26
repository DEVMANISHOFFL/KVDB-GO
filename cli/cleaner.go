package main

import "strings"

func Cleaner(text string) string {
	lowered := strings.ToLower(text)
	return lowered
}
