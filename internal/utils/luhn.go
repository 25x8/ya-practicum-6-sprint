package utils

import (
	"strconv"
)

func ValidateLuhn(number string) bool {
	digits := make([]int, 0, len(number))
	for _, r := range number {
		if r < '0' || r > '9' {
			return false 
		}
		digits = append(digits, int(r-'0'))
	}

	sum := 0
	parity := len(digits) % 2
	for i, digit := range digits {
		if i%2 == parity {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
	}

	return sum%10 == 0
}

func IsNumeric(s string) bool {
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}
