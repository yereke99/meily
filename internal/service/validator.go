package service

import (
	"errors"
	"fmt"
	"meily/config"
	"meily/internal/domain"
	"regexp"
	"strconv"
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
		return errors.New("price is not correct")
	}

	if pdfData.Bin != cfg.Bin {
		return errors.New("wrong bin number")
	}

	return nil
}
