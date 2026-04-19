package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SkillFile represents a skill stored as a SKILL.md file on disk.
type SkillFile struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content,omitempty"`
	Path        string `json:"path,omitempty"`
}

// SkillStore manages SKILL.md files under a base directory.
// Each skill lives at: {baseDir}/{slug}/SKILL.md
type SkillStore struct {
	baseDir string
}

func NewSkillStore(dir string) *SkillStore {
	return &SkillStore{baseDir: filepath.Join(dir, "skills")}
}

func (ss *SkillStore) ensureDir() error {
	return os.MkdirAll(ss.baseDir, 0755)
}

// List returns all skills (without content) from the base directory.
func (ss *SkillStore) List() ([]SkillFile, error) {
	entries, err := os.ReadDir(ss.baseDir)
	if os.IsNotExist(err) {
		return []SkillFile{}, nil
	}
	if err != nil {
		return nil, err
	}
	var skills []SkillFile
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		slug := e.Name()
		sf, err := ss.Get(slug)
		if err != nil {
			continue
		}
		sf.Content = "" // omit full content in list
		skills = append(skills, *sf)
	}
	if skills == nil {
		skills = []SkillFile{}
	}
	return skills, nil
}

// Get returns a skill with its full content.
func (ss *SkillStore) Get(slug string) (*SkillFile, error) {
	slug = sanitizeSkillSlug(slug)
	path := filepath.Join(ss.baseDir, slug, "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	name, desc := parseSkillFrontmatter(string(data))
	return &SkillFile{
		Slug:        slug,
		Name:        name,
		Description: desc,
		Content:     string(data),
		Path:        path,
	}, nil
}

// Save writes content to {baseDir}/{slug}/SKILL.md, creating dirs as needed.
func (ss *SkillStore) Save(slug, content string) error {
	if err := ss.ensureDir(); err != nil {
		return err
	}
	dir := filepath.Join(ss.baseDir, slug)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644)
}

// Delete removes a skill directory.
func (ss *SkillStore) Delete(slug string) error {
	slug = sanitizeSkillSlug(slug)
	return os.RemoveAll(filepath.Join(ss.baseDir, slug))
}

// InstallFromGitHub fetches SKILL.md files from a GitHub repo and saves them locally.
// Source formats: "owner/repo", "https://github.com/owner/repo", or with subpath.
func (ss *SkillStore) InstallFromGitHub(source string) (installed []string, errs []string) {
	owner, repo, subpath := parseGitHubSource(source)
	if owner == "" || repo == "" {
		return nil, []string{"invalid source: expected owner/repo or GitHub URL"}
	}

	// Fetch repo tree recursively from GitHub API
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/git/trees/HEAD?recursive=1", owner, repo)
	resp, err := http.Get(apiURL) //nolint:noctx
	if err != nil {
		return nil, []string{"failed to fetch repo tree: " + err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, []string{fmt.Sprintf("GitHub API returned %d for %s/%s", resp.StatusCode, owner, repo)}
	}

	var tree struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"tree"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tree); err != nil {
		return nil, []string{"failed to parse repo tree: " + err.Error()}
	}

	for _, item := range tree.Tree {
		if item.Type != "blob" {
			continue
		}
		// Match only SKILL.md files
		if !strings.HasSuffix(item.Path, "/SKILL.md") && item.Path != "SKILL.md" {
			continue
		}
		// Filter by subpath if specified
		if subpath != "" && !strings.HasPrefix(item.Path, subpath) {
			continue
		}

		rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/HEAD/%s", owner, repo, item.Path)
		content, err := fetchSkillFile(rawURL)
		if err != nil {
			errs = append(errs, fmt.Sprintf("failed to download %s: %s", item.Path, err))
			continue
		}

		name, _ := parseSkillFrontmatter(content)
		slug := skillSlugFromNameOrPath(name, item.Path)

		if err := ss.Save(slug, content); err != nil {
			errs = append(errs, fmt.Sprintf("failed to save %s: %s", slug, err))
			continue
		}
		installed = append(installed, slug)
	}

	if installed == nil {
		installed = []string{}
	}
	return
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func parseSkillFrontmatter(content string) (name, description string) {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "---") {
		// No frontmatter: extract name from first # heading
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "# ") {
				name = strings.TrimPrefix(line, "# ")
				return
			}
		}
		return
	}
	rest := trimmed[3:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return
	}
	fm := rest[:end]
	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "name:"); ok {
			name = strings.Trim(strings.TrimSpace(after), `"'`)
		} else if after, ok := strings.CutPrefix(line, "description:"); ok {
			description = strings.Trim(strings.TrimSpace(after), `"'`)
		}
	}
	return
}

var slugStripRe = regexp.MustCompile(`[^a-z0-9-]+`)

func sanitizeSkillSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugStripRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "skill"
	}
	return s
}

func skillSlugFromNameOrPath(name, path string) string {
	if name != "" {
		return sanitizeSkillSlug(name)
	}
	// Derive from parent directory of SKILL.md
	parts := strings.Split(strings.TrimSuffix(path, "/SKILL.md"), "/")
	return sanitizeSkillSlug(parts[len(parts)-1])
}

func parseGitHubSource(source string) (owner, repo, subpath string) {
	source = strings.TrimSpace(source)
	source = strings.TrimPrefix(source, "https://github.com/")
	source = strings.TrimPrefix(source, "http://github.com/")
	source = strings.TrimSuffix(source, ".git")

	parts := strings.SplitN(source, "/", 4)
	if len(parts) < 2 {
		return "", "", ""
	}
	owner = parts[0]
	repo = parts[1]
	if len(parts) >= 4 {
		// e.g. "tree/main/skills/my-skill" -> subpath = "skills/my-skill"
		if parts[2] == "tree" {
			subpath = parts[3]
		} else {
			subpath = strings.Join(parts[2:], "/")
		}
	}
	return
}

func fetchSkillFile(url string) (string, error) {
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	return string(data), err
}

// ── HTTP Handlers ─────────────────────────────────────────────────────────────

// @Summary List all skill files
// @Tags    skill-files
// @Security BearerAuth
// @Produce json
// @Success 200 {array} SkillFile
// @Failure 401 {object} ErrorResponse
// @Router  /v1/skill-files [get]
func (s *Server) handleListSkillFiles(w http.ResponseWriter, r *http.Request) {
	skills, err := s.skillStore.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list skills: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, skills)
}

// @Summary Get a skill file by slug
// @Tags    skill-files
// @Security BearerAuth
// @Produce json
// @Param   slug path string true "Skill slug"
// @Success 200 {object} SkillFile
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router  /v1/skill-files/{slug} [get]
func (s *Server) handleGetSkillFile(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	sf, err := s.skillStore.Get(slug)
	if os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "skill not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sf)
}

type createSkillFileRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

// @Summary Create a new skill file
// @Tags    skill-files
// @Security BearerAuth
// @Accept  json
// @Produce json
// @Param   body body createSkillFileRequest true "Skill name, description and optional content"
// @Success 201 {object} SkillFile
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router  /v1/skill-files [post]
func (s *Server) handleCreateSkillFile(w http.ResponseWriter, r *http.Request) {
	var req createSkillFileRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	slug := sanitizeSkillSlug(req.Name)
	content := req.Content
	if content == "" {
		content = fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n# %s\n\n%s\n",
			req.Name, req.Description, req.Name, req.Description)
	}

	if err := s.skillStore.Save(slug, content); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sf, _ := s.skillStore.Get(slug)
	writeJSON(w, http.StatusCreated, sf)
}

type updateSkillFileRequest struct {
	Content string `json:"content"`
}

// @Summary Update a skill file's content
// @Tags    skill-files
// @Security BearerAuth
// @Accept  json
// @Produce json
// @Param   slug path string               true "Skill slug"
// @Param   body body updateSkillFileRequest true "New content"
// @Success 200 {object} SkillFile
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router  /v1/skill-files/{slug} [put]
func (s *Server) handleUpdateSkillFile(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	var req updateSkillFileRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := s.skillStore.Save(slug, req.Content); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sf, _ := s.skillStore.Get(slug)
	writeJSON(w, http.StatusOK, sf)
}

// @Summary Delete a skill file by slug
// @Tags    skill-files
// @Security BearerAuth
// @Produce json
// @Param   slug path string true "Skill slug"
// @Success 200 {object} map[string]bool
// @Failure 401 {object} ErrorResponse
// @Router  /v1/skill-files/{slug} [delete]
func (s *Server) handleDeleteSkillFile(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if err := s.skillStore.Delete(slug); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

type installSkillsRequest struct {
	Source string `json:"source"`
}

type installSkillsResult struct {
	Installed []string `json:"installed"`
	Errors    []string `json:"errors,omitempty"`
	Count     int      `json:"count"`
}

// @Summary Install skill files from a GitHub repository
// @Tags    skill-files
// @Security BearerAuth
// @Accept  json
// @Produce json
// @Param   body body installSkillsRequest true "GitHub source (owner/repo or URL)"
// @Success 200 {object} installSkillsResult
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router  /v1/skill-files/install [post]
func (s *Server) handleInstallSkills(w http.ResponseWriter, r *http.Request) {
	var req installSkillsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Source == "" {
		writeError(w, http.StatusBadRequest, "source is required")
		return
	}
	installed, errs := s.skillStore.InstallFromGitHub(req.Source)
	writeJSON(w, http.StatusOK, installSkillsResult{
		Installed: installed,
		Errors:    errs,
		Count:     len(installed),
	})
}
