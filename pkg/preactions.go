package binman

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/google/go-github/v50/github"
	"github.com/rjbrown57/binman/pkg/gh"
	"github.com/rjbrown57/binman/pkg/gl"
	log "github.com/rjbrown57/binman/pkg/logging"
	"github.com/rjbrown57/binman/pkg/templating"
	"github.com/xanzy/go-gitlab"
)

type ReleaseStatusAction struct {
	r           *BinmanRelease
	releasePath string
}

func (r *BinmanRelease) AddReleaseStatusAction(releasePath string) Action {
	return &ReleaseStatusAction{
		r,
		releasePath,
	}
}

// ReleaseStatusAction verifies whether we have work to do
func (action *ReleaseStatusAction) execute() error {

	action.r.setpublishPath(action.releasePath, action.r.Version)
	_, err := os.Stat(action.r.PublishPath)

	if action.r.watchExposeMetrics {
		var latestLabel string = "true"
		if action.r.QueryType == "releasebytag" {
			latestLabel = "false"
		}
		action.r.metric.WithLabelValues(latestLabel, action.r.SourceIdentifier, action.r.Repo, action.r.Version)
	}

	// If err nil we already have this version, send custom error so gosyncrepo knows to end actions
	// Default to capture any other error cases
	switch err {
	case nil:
		return fmt.Errorf("%s", "Noupdate")
	default:
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
}

type SetUrlAction struct {
	r *BinmanRelease
}

func (r *BinmanRelease) AddSetUrlAction() Action {
	return &SetUrlAction{
		r,
	}
}

// format a user specified url with release data
func (action *SetUrlAction) execute() error {

	// If user has set an external url use that to grab target
	if action.r.ExternalUrl != "" {
		log.Debugf("User specified url %s", action.r.dlUrl)
		action.r.dlUrl = templating.TemplateString(action.r.ExternalUrl, action.r.getDataMap())
		action.r.assetName = filepath.Base(action.r.dlUrl)
		return nil
	}

	switch data := action.r.relData.(type) {
	case *github.RepositoryRelease:
		// If the user has requested a specifc asset check for that
		if action.r.ReleaseFileName != "" {
			rFilename := templating.TemplateString(action.r.ReleaseFileName, action.r.getDataMap())
			log.Debugf("Get gh asset by name %s", rFilename)
			action.r.assetName, action.r.dlUrl = gh.GetAssetbyName(rFilename, data.Assets)
		} else {
			// Attempt to find the asset via arch/os
			log.Debugf("Attempt to find github asset for %s", action.r.project)
			action.r.assetName, action.r.dlUrl = selectAsset(action.r.Arch, action.r.Os, action.r.Version, action.r.project, gh.GHGetAssetData(data.Assets))
		}
	case []*gitlab.ReleaseLink:
		// If the user has requested a specifc asset check for that
		if action.r.ReleaseFileName != "" {
			rFilename := templating.TemplateString(action.r.ReleaseFileName, action.r.getDataMap())
			log.Debugf("Get gl asset by name %s", rFilename)
			action.r.assetName, action.r.dlUrl = gl.GetAssetbyName(rFilename, data)
		} else {
			// Attempt to find the asset via arch/os
			log.Debugf("Attempt to find gitlab asset for %s\n", action.r.project)
			action.r.assetName, action.r.dlUrl = selectAsset(action.r.Arch, action.r.Os, action.r.Version, action.r.project, gl.GLGetAssetData(data))
		}
	}

	// If at this point dlUrl is not set we have an issue
	if action.r.dlUrl == "" {
		return fmt.Errorf("Target release asset not found for %s", action.r.Repo)
	}

	return nil
}

type SetArtifactPathAction struct {
	r           *BinmanRelease
	releasePath string
	binPath     string
}

func (r *BinmanRelease) AddSetArtifactPathAction(releasePath, binPath string) Action {
	return &SetArtifactPathAction{
		r,
		releasePath,
		binPath,
	}
}

func (action *SetArtifactPathAction) execute() error {
	action.r.setArtifactPath(action.releasePath, action.binPath, action.r.assetName)
	// We set cleanupOnFailure to true in case we hit an issue further down the line
	action.r.cleanupOnFailure = true
	// If the binPath string is empty, for example when using `binman get`, don't create the directory
	if action.binPath != "" {
		if err := CreateDirectory(action.binPath); err != nil {
			return err
		}
	}
	err := CreateDirectory(action.r.PublishPath)
	// At this point we have created something during the release process
	return err
}
