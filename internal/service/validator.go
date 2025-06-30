// First, enhance the service package with custom error types
package service

import (
	"errors"
	"fmt"
	"meily/config"
	"meily/internal/domain"
	"regexp"
	"strconv"
)

// Custom error types for better error handling
var (
	ErrWrongPrice = errors.New("price is not correct")
	ErrWrongBin   = errors.New("wrong bin number")
)

func ParsePrice(raw string) (int, error) {
	// Убираем все, кроме цифр
	re := regexp.MustCompile(`\D+`)
	digits := re.ReplaceAllString(raw, "")
	if digits == "" {
		return 0, fmt.Errorf("no digits found in price %q", raw)
	}
	return strconv.Atoi(digits)
}

func Validator(cfg *config.Config, pdfData domain.PdfResult) error {
	mustPrice := pdfData.Total * cfg.Cost
	if pdfData.ActualPrice != mustPrice {
		return ErrWrongPrice
	}

	if pdfData.Bin != cfg.Bin {
		return ErrWrongBin
	}

	return nil
}

// Alternative approach with detailed error info
type ValidationError struct {
	Type    string
	Message string
	Details map[string]interface{}
}

func (e ValidationError) Error() string {
	return e.Message
}

func ValidatorWithDetails(cfg *config.Config, pdfData domain.PdfResult) error {
	mustPrice := pdfData.Total * cfg.Cost
	if pdfData.ActualPrice != mustPrice {
		return ValidationError{
			Type:    "wrong_price",
			Message: "price is not correct",
			Details: map[string]interface{}{
				"expected": mustPrice,
				"actual":   pdfData.ActualPrice,
			},
		}
	}

	if pdfData.Bin != cfg.Bin {
		return ValidationError{
			Type:    "wrong_bin",
			Message: "wrong bin number",
			Details: map[string]interface{}{
				"expected": cfg.Bin,
				"actual":   pdfData.Bin,
			},
		}
	}

	return nil
}
