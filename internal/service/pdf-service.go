package service

import (
	"fmt"
	"os"
	"strings"

	"github.com/unidoc/unipdf/v3/extractor"
	"github.com/unidoc/unipdf/v3/model"
)

// ReadPDF reads a PDF file and returns all text content as []string
// Each element represents a line from the PDF
func ReadPDF(filePath string) ([]string, error) {
	// Open the PDF file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open PDF file: %w", err)
	}
	defer file.Close()

	// Create PDF reader
	pdfReader, err := model.NewPdfReader(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create PDF reader: %w", err)
	}

	var allText []string

	// Get number of pages
	numPages, err := pdfReader.GetNumPages()
	if err != nil {
		return nil, fmt.Errorf("failed to get number of pages: %w", err)
	}

	// Extract text from each page
	for pageNum := 1; pageNum <= numPages; pageNum++ {
		page, err := pdfReader.GetPage(pageNum)
		if err != nil {
			fmt.Printf("Warning: failed to get page %d: %v\n", pageNum, err)
			continue
		}

		// Create text extractor
		ex, err := extractor.New(page)
		if err != nil {
			fmt.Printf("Warning: failed to create extractor for page %d: %v\n", pageNum, err)
			continue
		}

		// Extract text from page
		text, err := ex.ExtractText()
		if err != nil {
			fmt.Printf("Warning: failed to extract text from page %d: %v\n", pageNum, err)
			continue
		}

		// Split text into lines and add to result
		lines := splitTextIntoLines(text)
		allText = append(allText, lines...)
	}

	return cleanLines(allText), nil
}

// splitTextIntoLines splits text into individual lines
func splitTextIntoLines(text string) []string {
	lines := strings.Split(text, "\n")
	var result []string

	for _, line := range lines {
		cleaned := strings.TrimSpace(line)
		if cleaned != "" {
			result = append(result, cleaned)
		}
	}

	return result
}

// cleanLines removes empty lines and extra whitespace
func cleanLines(lines []string) []string {
	var cleaned []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}

	return cleaned
}
