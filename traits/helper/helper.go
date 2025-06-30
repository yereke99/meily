package helper

import "fmt"

// Helper function to format price with thousand separators
func FormatPrice(price int) string {
	str := fmt.Sprintf("%d", price)
	if len(str) <= 3 {
		return str
	}

	var result []rune
	for i, char := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result = append(result, ' ')
		}
		result = append(result, char)
	}
	return string(result)
}
