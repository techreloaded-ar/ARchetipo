package connector

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

const planSeparator = "\n\n---\n\n"

func SplitPlanSections(body string) (specBody, planBody string) {
	body = strings.TrimSpace(body)
	if strings.HasPrefix(body, "---\n") {
		return "", strings.TrimSpace(strings.TrimPrefix(body, "---\n"))
	}
	if index := strings.Index(body, "\n---\n"); index >= 0 {
		return strings.TrimSpace(body[:index]), strings.TrimSpace(body[index+len("\n---\n"):])
	}
	return body, ""
}

func JoinPlanSections(specBody, planBody string) string {
	specBody = strings.TrimSpace(specBody)
	planBody = strings.TrimSpace(planBody)
	if planBody == "" {
		return specBody
	}
	if specBody == "" {
		return "---\n\n" + planBody
	}
	return specBody + planSeparator + planBody
}

func FirstNonEmpty(first, second string) string {
	if strings.TrimSpace(first) != "" {
		return first
	}
	return second
}

func SaveLocalPRD(path, content string) (domain.WriteResult, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return domain.WriteResult{}, fmt.Errorf("creating dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return domain.WriteResult{}, fmt.Errorf("writing %s: %w", path, err)
	}
	return domain.WriteResult{OK: true, Refs: []domain.Ref{{Path: path}}}, nil
}
