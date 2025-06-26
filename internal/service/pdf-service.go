package service

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ReadPDFWithPython reads a PDF file using Python script and returns text content as []string
func ReadPDFWithPython(filePath string) ([]string, error) {
	// Get absolute path to ensure Python script can find the file
	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Check if file exists
	if _, err := os.Stat(absFilePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("PDF file does not exist: %s", absFilePath)
	}

	// Get the directory where the Go binary is running
	workDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	// Path to Python script (relative to project root)
	pythonScriptPath := filepath.Join(workDir, "internal", "service", "pdfReader.py")

	// Check if Python script exists
	if _, err := os.Stat(pythonScriptPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("Python script not found: %s", pythonScriptPath)
	}

	// Prepare the command
	cmd := exec.Command("python3.8", pythonScriptPath, absFilePath)

	// Set working directory for the command
	cmd.Dir = workDir

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to execute Python script: %w\nOutput: %s", err, string(output))
	}

	// Convert output to string and process
	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" {
		return []string{}, nil
	}

	// Parse the Python output (assuming it's a Python list format)
	lines, err := parsePythonListOutput(outputStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Python output: %w", err)
	}

	return lines, nil
}

// parsePythonListOutput parses Python list output format like ['item1', 'item2', ...]
func parsePythonListOutput(output string) ([]string, error) {
	// Remove leading/trailing whitespace
	output = strings.TrimSpace(output)

	// Check if output starts and ends with brackets
	if !strings.HasPrefix(output, "[") || !strings.HasSuffix(output, "]") {
		// If it's not a proper list format, try to split by newlines
		lines := strings.Split(output, "\n")
		var result []string
		for _, line := range lines {
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				result = append(result, trimmed)
			}
		}
		return result, nil
	}

	// Try to parse as JSON array first
	var jsonResult []string
	if err := json.Unmarshal([]byte(output), &jsonResult); err == nil {
		return jsonResult, nil
	}

	// Fallback: manual parsing of Python list format
	// Remove brackets
	content := strings.TrimPrefix(output, "[")
	content = strings.TrimSuffix(content, "]")

	if content == "" {
		return []string{}, nil
	}

	// Split by comma, but be careful with quoted strings
	var result []string
	var current strings.Builder
	inQuotes := false
	quoteChar := byte(0)

	for i := 0; i < len(content); i++ {
		char := content[i]

		if !inQuotes && (char == '\'' || char == '"') {
			// Starting a quoted string
			inQuotes = true
			quoteChar = char
		} else if inQuotes && char == quoteChar {
			// Check if it's an escaped quote
			if i > 0 && content[i-1] != '\\' {
				// Ending a quoted string
				inQuotes = false
				quoteChar = 0
			} else {
				current.WriteByte(char)
			}
		} else if !inQuotes && char == ',' {
			// Found a separator
			item := strings.TrimSpace(current.String())
			if item != "" {
				// Remove surrounding quotes if present
				if (strings.HasPrefix(item, "'") && strings.HasSuffix(item, "'")) ||
					(strings.HasPrefix(item, "\"") && strings.HasSuffix(item, "\"")) {
					item = item[1 : len(item)-1]
				}
				result = append(result, item)
			}
			current.Reset()
		} else {
			current.WriteByte(char)
		}
	}

	// Add the last item
	if current.Len() > 0 {
		item := strings.TrimSpace(current.String())
		if item != "" {
			// Remove surrounding quotes if present
			if (strings.HasPrefix(item, "'") && strings.HasSuffix(item, "'")) ||
				(strings.HasPrefix(item, "\"") && strings.HasSuffix(item, "\"")) {
				item = item[1 : len(item)-1]
			}
			result = append(result, item)
		}
	}

	return result, nil
}

// ReadPDFWithPythonAlternative - Alternative approach with JSON output
func ReadPDFWithPythonAlternative(filePath string) ([]string, error) {
	// Get absolute path
	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Check if file exists
	if _, err := os.Stat(absFilePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("PDF file does not exist: %s", absFilePath)
	}

	// Create a temporary Python script that outputs JSON
	tempScript := `
import sys
import json
sys.path.append('internal/service')
from pdfReader import PDFReaders

if len(sys.argv) != 2:
    print(json.dumps(["Error: No file path provided"]))
    sys.exit(1)

file_path = sys.argv[1]
try:
    pdf_reader = PDFReaders(file_path)
    pdf_reader.open_pdf()
    detailed_info = pdf_reader.extract_detailed_info()
    pdf_reader.close_pdf()
    print(json.dumps(detailed_info, ensure_ascii=False))
except Exception as e:
    print(json.dumps([f"Error: {str(e)}"]))
`

	// Write temporary script
	tempFile, err := os.CreateTemp("", "pdf_reader_*.py")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	if _, err := tempFile.WriteString(tempScript); err != nil {
		return nil, fmt.Errorf("failed to write temp script: %w", err)
	}
	tempFile.Close()

	// Execute the temporary script
	cmd := exec.Command("python3", tempFile.Name(), absFilePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to execute Python script: %w\nOutput: %s", err, string(output))
	}

	// Parse JSON output
	var result []string
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON output: %w\nOutput: %s", err, string(output))
	}

	return result, nil
}

// ReadPDF - Main function that tries both approaches
func ReadPDF(filePath string) ([]string, error) {
	// Try the direct Python script approach first
	result, err := ReadPDFWithPython(filePath)
	if err != nil {
		// Fallback to alternative approach
		return ReadPDFWithPythonAlternative(filePath)
	}
	return result, nil
}
