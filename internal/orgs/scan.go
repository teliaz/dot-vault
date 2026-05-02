package orgs

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/teliaz/dot-vault/internal/config"
)

type Repository struct {
	Name           string
	RelPath        string
	AbsPath        string
	EnvFiles       []EnvFile
	SampleEnvFiles []EnvFile
}

type EnvFile struct {
	Name      string
	RelPath   string
	AbsPath   string
	Size      int64
	UpdatedAt time.Time
}

func (s *Service) ResolveOrganization(name string) (config.Organization, error) {
	cfg, err := s.configManager.Load()
	if err != nil {
		return config.Organization{}, err
	}

	targetName := strings.TrimSpace(name)
	if targetName == "" {
		targetName = cfg.ActiveOrganization
	}
	if targetName == "" {
		return config.Organization{}, fmt.Errorf("organization is required; no active organization is configured")
	}

	org, ok := cfg.Organizations[targetName]
	if !ok {
		return config.Organization{}, fmt.Errorf("organization %q not found", targetName)
	}
	return org, nil
}

func (s *Service) Scan(_ context.Context, organization string) ([]Repository, error) {
	org, err := s.ResolveOrganization(organization)
	if err != nil {
		return nil, err
	}

	repoByPath := map[string]Repository{}
	err = filepath.WalkDir(org.RepoRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == org.RepoRoot {
			return nil
		}
		if entry.IsDir() && shouldSkipScanDir(entry.Name()) {
			return filepath.SkipDir
		}
		if entry.Name() != ".git" {
			return nil
		}

		repoRoot := filepath.Dir(path)
		relPath, err := filepath.Rel(org.RepoRoot, repoRoot)
		if err != nil {
			return fmt.Errorf("resolve repo relative path: %w", err)
		}
		envFiles, err := discoverEnvFiles(repoRoot)
		if err != nil {
			return err
		}
		sampleEnvFiles, err := discoverSampleEnvFiles(repoRoot)
		if err != nil {
			return err
		}
		repoByPath[relPath] = Repository{
			Name:           filepath.Base(repoRoot),
			RelPath:        filepath.ToSlash(relPath),
			AbsPath:        repoRoot,
			EnvFiles:       envFiles,
			SampleEnvFiles: sampleEnvFiles,
		}

		if entry.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan repositories: %w", err)
	}

	repos := make([]Repository, 0, len(repoByPath))
	for _, repo := range repoByPath {
		repos = append(repos, repo)
	}
	sort.Slice(repos, func(i, j int) bool {
		return repos[i].RelPath < repos[j].RelPath
	})

	return repos, nil
}

func shouldSkipScanDir(name string) bool {
	switch name {
	case "node_modules", "vendor", ".cache", ".tmp":
		return true
	default:
		return false
	}
}

func discoverEnvFiles(repoRoot string) ([]EnvFile, error) {
	entries, err := os.ReadDir(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("read repo root: %w", err)
	}

	candidates := map[string]fs.DirEntry{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if isDefaultEnvFile(entry.Name()) {
			candidates[entry.Name()] = entry
		}
	}

	gitignorePatterns, err := readGitignoreEnvPatterns(repoRoot)
	if err != nil {
		return nil, err
	}
	for _, pattern := range gitignorePatterns {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if matchesGitignoreEnvPattern(pattern, entry.Name()) && !isSampleEnvFile(entry.Name()) {
				candidates[entry.Name()] = entry
			}
		}
	}

	return envFilesFromCandidates(repoRoot, candidates)
}

func discoverSampleEnvFiles(repoRoot string) ([]EnvFile, error) {
	entries, err := os.ReadDir(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("read repo root: %w", err)
	}

	candidates := map[string]fs.DirEntry{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if isSampleEnvFile(entry.Name()) {
			candidates[entry.Name()] = entry
		}
	}

	return envFilesFromCandidates(repoRoot, candidates)
}

func envFilesFromCandidates(repoRoot string, candidates map[string]fs.DirEntry) ([]EnvFile, error) {
	envFiles := make([]EnvFile, 0, len(candidates))
	for name, entry := range candidates {
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("stat env file %s: %w", name, err)
		}
		envFiles = append(envFiles, EnvFile{
			Name:      name,
			RelPath:   name,
			AbsPath:   filepath.Join(repoRoot, name),
			Size:      info.Size(),
			UpdatedAt: info.ModTime(),
		})
	}
	sort.Slice(envFiles, func(i, j int) bool {
		return envFiles[i].Name < envFiles[j].Name
	})

	return envFiles, nil
}

func isDefaultEnvFile(name string) bool {
	if name == ".env" {
		return true
	}
	if !strings.HasPrefix(name, ".env.") {
		return false
	}

	lowerName := strings.ToLower(name)
	for _, suffix := range []string{".example", ".sample", ".template"} {
		if strings.HasSuffix(lowerName, suffix) {
			return false
		}
	}
	return true
}

func isSampleEnvFile(name string) bool {
	lowerName := strings.ToLower(name)
	switch lowerName {
	case ".env_sample", ".env-template", ".env-example", "sample.env", "example.env", "env.sample", "env.template":
		return true
	}
	if !strings.HasPrefix(lowerName, ".env.") {
		return false
	}
	for _, suffix := range []string{".example", ".sample", ".template"} {
		if strings.HasSuffix(lowerName, suffix) {
			return true
		}
	}
	return false
}

func SuggestedEnvFileName(sampleName string) string {
	lowerName := strings.ToLower(sampleName)
	switch lowerName {
	case ".env_sample", ".env-template", ".env-example", "sample.env", "example.env", "env.sample", "env.template":
		return ".env"
	}
	if strings.HasPrefix(lowerName, ".env.") {
		for _, suffix := range []string{".example", ".sample", ".template"} {
			if strings.HasSuffix(lowerName, suffix) {
				return sampleName[:len(sampleName)-len(suffix)]
			}
		}
	}
	return ".env"
}

func readGitignoreEnvPatterns(repoRoot string) ([]string, error) {
	payload, err := os.ReadFile(filepath.Join(repoRoot, ".gitignore"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read .gitignore: %w", err)
	}

	var patterns []string
	for _, line := range strings.Split(string(payload), "\n") {
		pattern := strings.TrimSpace(line)
		if pattern == "" || strings.HasPrefix(pattern, "#") || strings.HasPrefix(pattern, "!") {
			continue
		}
		if strings.Contains(pattern, ".env") {
			patterns = append(patterns, strings.TrimPrefix(pattern, "/"))
		}
	}
	return patterns, nil
}

func matchesGitignoreEnvPattern(pattern string, name string) bool {
	pattern = filepath.Base(strings.TrimSuffix(pattern, "/"))
	if pattern == "" {
		return false
	}
	matched, err := filepath.Match(pattern, name)
	if err == nil && matched {
		return true
	}
	return pattern == name
}
