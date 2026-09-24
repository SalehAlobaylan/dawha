package trees

import (
	"errors"
	"time"
)

type VersionState string

const (
	Draft     VersionState = "draft"
	Published VersionState = "published"
	Archived  VersionState = "archived"
)

type Version struct {
	Number          int
	State           VersionState
	PublicationNote string
	PublishedBy     string
	PublishedAt     time.Time
}

var (
	ErrNotDraft         = errors.New("only a draft version can be published")
	ErrMissingActor     = errors.New("publisher is required")
	ErrAlreadyPublished = errors.New("published versions are immutable")
)

func NewDraft(number int) Version {
	return Version{Number: number, State: Draft}
}

func Publish(version *Version, actor string, note string, now time.Time) error {
	if version == nil {
		return ErrNotDraft
	}
	if version.State == Published || version.State == Archived {
		return ErrAlreadyPublished
	}
	if version.State != Draft {
		return ErrNotDraft
	}
	if actor == "" {
		return ErrMissingActor
	}
	version.State = Published
	version.PublishedBy = actor
	version.PublicationNote = note
	version.PublishedAt = now
	return nil
}
