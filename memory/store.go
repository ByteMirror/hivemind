package memory

// MemoryStore defines the local memory operations surface used by higher layers.
// It intentionally maps to the current Manager API so future daemon/remote
// adapters can satisfy the same contract.
type MemoryStore interface {
	Dir() string
	GitEnabled() bool

	Search(query string, opts SearchOpts) ([]SearchResult, error)
	Read(relPath string) (string, error)
	ReadAtRef(relPath, ref string) (string, error)
	Get(relPath string, from, lines int) (string, error)
	GetAtRef(relPath string, from, lines int, ref string) (string, error)
	List() ([]FileInfo, error)
	ListAtRef(ref string) ([]FileInfo, error)
	Tree() ([]TreeEntry, error)
	TreeAtRef(ref string) ([]TreeEntry, error)
	History(relPath string, count int) ([]GitLogEntry, error)
	HistoryWithBranch(relPath string, count int, branch string) ([]GitLogEntry, error)

	WriteWithCommitMessage(content, file, commitMsg string) error
	WriteWithCommitMessageOnBranch(content, file, commitMsg, branch string) error
	WriteFile(relPath, content, commitMsg string) error
	WriteFileOnBranch(relPath, content, commitMsg, branch string) error
	Append(relPath, content string) error
	AppendOnBranch(relPath, content, branch string) error
	Move(from, to string) error
	MoveOnBranch(from, to, branch string) error
	Delete(relPath string) error
	DeleteOnBranch(relPath, branch string) error
	Pin(relPath string) error
	PinOnBranch(relPath, branch string) error
	Unpin(relPath string) error
	UnpinOnBranch(relPath, branch string) error

	GitBranches() (current string, branches []string, err error)
	GitBranchInfo() (GitBranchInfo, error)
	CreateBranch(name, fromRef string) error
	DeleteBranch(name string, force bool) error
	MergeBranch(source, target, strategy string) error
	DiffRefs(baseRef, headRef, relPath string) (string, error)
}

// Ensure Manager satisfies the MemoryStore contract.
var _ MemoryStore = (*Manager)(nil)
