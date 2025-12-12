package runner

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

// RunnerInfo holds information about a discovered runner image
type RunnerInfo struct {
	Image    string
	Language string
}

// Registry manages available runner images
type Registry struct {
	runnersByLanguage map[string]RunnerInfo
}

// NewRegistry creates a new registry by discovering runner images from Docker
func NewRegistry(ctx context.Context, cli *client.Client) (*Registry, error) {
	// List images with label sandbox.runner=true
	filterArgs := filters.NewArgs()
	filterArgs.Add("label", "sandbox.runner=true")

	images, err := cli.ImageList(ctx, image.ListOptions{
		Filters: filterArgs,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list docker images: %w", err)
	}

	runnersByLanguage := make(map[string]RunnerInfo)

	for _, img := range images {
		// Extract language from labels
		language, ok := img.Labels["sandbox.language"]
		if !ok || language == "" {
			continue
		}

		// Use first RepoTag as image name, or ID if no tags
		imageName := img.ID
		if len(img.RepoTags) > 0 {
			imageName = img.RepoTags[0]
		}

		runnersByLanguage[language] = RunnerInfo{
			Image:    imageName,
			Language: language,
		}
	}

	return &Registry{
		runnersByLanguage: runnersByLanguage,
	}, nil
}

// GetRunner returns the runner info for a given language
func (r *Registry) GetRunner(language string) (RunnerInfo, bool) {
	runner, ok := r.runnersByLanguage[language]
	return runner, ok
}

// ListRunners returns all available runners
func (r *Registry) ListRunners() []RunnerInfo {
	runners := make([]RunnerInfo, 0, len(r.runnersByLanguage))
	for _, runner := range r.runnersByLanguage {
		runners = append(runners, runner)
	}
	return runners
}
