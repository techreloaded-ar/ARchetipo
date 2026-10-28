package verification

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Logs presents bounded preparation output without reading arbitrary local
// paths from a supplied report. All command logs belong to this checkout.
func (s *Service) Logs(result Result, limit int) (string, error) {
	if limit <= 0 || limit > 64000 {
		limit = 16000
	}
	var output strings.Builder
	if result.ReusedReport != "" {
		fmt.Fprintf(&output, "Preparazione riutilizzata: %s\n", result.ReusedReport)
	}
	for _, step := range result.Steps {
		rel, err := filepath.Rel(s.Root, step.LogPath)
		if err != nil {
			return "", err
		}
		allowed := strings.HasPrefix(filepath.ToSlash(rel), ".archetipo/environment/runs/")
		if !allowed && strings.HasPrefix(result.Operation, "check:") && s.Results != "" {
			results := s.Results
			if !filepath.IsAbs(results) {
				results = filepath.Join(s.Root, results)
			}
			resultsRel, resultsErr := filepath.Rel(s.Root, results)
			if resultsErr == nil {
				if _, pathErr := localPath(s.Root, filepath.ToSlash(resultsRel)); pathErr == nil {
					allowed = strings.HasPrefix(filepath.ToSlash(rel), filepath.ToSlash(resultsRel)+"/")
				}
			}
		}
		if !allowed {
			return "", invalid("environment log is outside this checkout's reports")
		}
		path, err := localPath(s.Root, filepath.ToSlash(rel))
		if err != nil {
			return "", err
		}
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		info, err := file.Stat()
		if err == nil && info.Size() > int64(limit) {
			_, err = file.Seek(-int64(limit), io.SeekEnd)
		}
		if err != nil {
			_ = file.Close()
			return "", err
		}
		raw, err := io.ReadAll(io.LimitReader(file, int64(limit)))
		closeErr := file.Close()
		if err != nil {
			return "", err
		}
		if closeErr != nil {
			return "", closeErr
		}
		fmt.Fprintf(&output, "%s · exit %d\n%s\n", step.Name, step.ExitCode, raw)
		if output.Len() > limit {
			value := output.String()
			output.Reset()
			output.WriteString(value[len(value)-limit:])
		}
	}
	return output.String(), nil
}
