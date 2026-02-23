package session

import (
	"time"

	"github.com/ByteMirror/hivemind/session/git"
)

// TopicData represents the serializable data of a Topic.
type TopicData struct {
	Name         string            `json:"name"`
	WorktreeMode TopicWorktreeMode `json:"worktree_mode,omitempty"`
	// SharedWorktree is kept for backwards-compatible JSON deserialization.
	// New writes use WorktreeMode instead.
	SharedWorktree bool            `json:"shared_worktree,omitempty"`
	AutoYes        bool            `json:"auto_yes"`
	Branch         string          `json:"branch,omitempty"`
	Path           string          `json:"path"`
	CreatedAt      time.Time       `json:"created_at"`
	Worktree       GitWorktreeData `json:"worktree,omitempty"`
	Notes          string          `json:"notes,omitempty"`
	Tasks          []TopicTask     `json:"tasks,omitempty"`
}

// ToTopicData converts a Topic to its serializable form.
func (t *Topic) ToTopicData() TopicData {
	data := TopicData{
		Name:         t.Name,
		WorktreeMode: t.WorktreeMode,
		AutoYes:      t.AutoYes,
		Branch:       t.Branch,
		Path:         t.Path,
		CreatedAt:    t.CreatedAt,
		Notes:        t.Notes,
		Tasks:        t.Tasks,
	}
	if t.gitWorktree != nil {
		data.Worktree = GitWorktreeData{
			RepoPath:      t.gitWorktree.GetRepoPath(),
			WorktreePath:  t.gitWorktree.GetWorktreePath(),
			SessionName:   t.Name,
			BranchName:    t.gitWorktree.GetBranchName(),
			BaseCommitSHA: t.gitWorktree.GetBaseCommitSHA(),
		}
	}
	return data
}

// FromTopicData creates a Topic from serialized data.
func FromTopicData(data TopicData) *Topic {
	// Migrate legacy SharedWorktree bool to WorktreeMode.
	mode := data.WorktreeMode
	if mode == "" {
		if data.SharedWorktree {
			mode = TopicWorktreeModeShared
		} else {
			mode = TopicWorktreeModePerInstance
		}
	}
	topic := &Topic{
		Name:         data.Name,
		WorktreeMode: mode,
		AutoYes:      data.AutoYes,
		Branch:       data.Branch,
		Path:         data.Path,
		CreatedAt:    data.CreatedAt,
		Notes:        data.Notes,
		Tasks:        data.Tasks,
		started:      true,
	}
	if mode == TopicWorktreeModeShared && data.Worktree.WorktreePath != "" {
		topic.gitWorktree = git.NewGitWorktreeFromStorage(
			data.Worktree.RepoPath,
			data.Worktree.WorktreePath,
			data.Worktree.SessionName,
			data.Worktree.BranchName,
			data.Worktree.BaseCommitSHA,
		)
	}
	return topic
}
