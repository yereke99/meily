package service

import (
	"errors"
	"meily/config"
	"meily/internal/domain"
	"strconv"
	"strings"
)

func ParsePrice(priceStr string) (int, error) {
	s := strings.ReplaceAll(priceStr, "₸", "")
	s = strings.ReplaceAll(s, "", "")
	return strconv.Atoi(s)
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
