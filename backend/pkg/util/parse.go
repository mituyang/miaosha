package util

import "strconv"

// ParseUint64 字符串转 uint64
func ParseUint64(s string) (uint64, error) {
	return strconv.ParseUint(s, 10, 64)
}
