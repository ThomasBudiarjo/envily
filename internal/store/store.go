// Package store defines Envily's persistence interface and domain models.
// Implementations live in store/sqlite and store/postgres. The store knows
// nothing about encryption: it stores ciphertext bytes as given.
package store

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrNotFound means the row does not exist (or a referenced row does not).
	ErrNotFound = errors.New("not found")
	// ErrConflict means a unique constraint was violated.
	ErrConflict = errors.New("conflict")
)

// Meta keys.
const (
	MetaKeyCheck       = "key_check"
	MetaRootKeyVersion = "root_key_version"
)

type User struct {
	ID                 string
	Email              string
	Name               string
	PasswordHash       string
	IsAdmin            bool
	MustChangePassword bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DisabledAt         *time.Time
}

type Session struct {
	ID         string
	UserID     string
	TokenHash  []byte
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
	IP         string
	UserAgent  string
}

type Project struct {
	ID             string
	Name           string
	Slug           string
	WrappedDEK     []byte
	DEKRootVersion int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Role string

const (
	RoleOwner  Role = "owner"
	RoleEditor Role = "editor"
	RoleViewer Role = "viewer"
)

type Member struct {
	ProjectID string
	UserID    string
	Role      Role
	CreatedAt time.Time
	// Email and Name are filled in by ListMembers.
	Email string
	Name  string
}

type Environment struct {
	ID        string
	ProjectID string
	Name      string
	Slug      string
	Position  int64
	CreatedAt time.Time
}

type Secret struct {
	ID             string
	EnvironmentID  string
	Key            string
	CurrentVersion int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time
}

type SecretVersion struct {
	ID        string
	SecretID  string
	Version   int64
	ValueCT   []byte
	CommentCT []byte // nil when the version has no comment
	ActorType string
	ActorID   string
	CreatedAt time.Time
}

// LiveSecret is a secret together with its current version.
type LiveSecret struct {
	Secret
	ValueCT   []byte
	CommentCT []byte
	ActorType string
	ActorID   string
}

// SecretKey identifies a live secret without its value.
type SecretKey struct {
	EnvironmentID  string
	Key            string
	CurrentVersion int64
	UpdatedAt      time.Time
}

type Permission string

const (
	PermRead      Permission = "read"
	PermReadWrite Permission = "read_write"
)

type ServiceToken struct {
	ID             string
	ProjectID      string
	Name           string
	TokenHash      []byte
	Permission     Permission
	CreatedBy      string
	ExpiresAt      *time.Time
	LastUsedAt     *time.Time
	RevokedAt      *time.Time
	CreatedAt      time.Time
	EnvironmentIDs []string
}

type PersonalToken struct {
	ID         string
	UserID     string
	Name       string
	TokenHash  []byte
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

type AuditLog struct {
	ID            string
	At            time.Time
	ActorType     string
	ActorID       string
	ActorLabel    string
	Action        string
	ProjectID     string // empty when not project-scoped
	EnvironmentID string // empty when not environment-scoped
	TargetType    string
	TargetID      string
	Metadata      string // JSON object
	IP            string
	UserAgent     string
}

// AuditCursor marks the last row of a page; the next page starts after it.
type AuditCursor struct {
	At time.Time
	ID string
}

// AuditFilter selects audit rows, newest first. Zero fields do not filter.
type AuditFilter struct {
	ProjectID string
	Action    string
	ActorID   string
	From, To  time.Time
	After     *AuditCursor
	Limit     int
}

type Users interface {
	Create(ctx context.Context, u User) error
	Get(ctx context.Context, id string) (User, error)
	GetByEmail(ctx context.Context, email string) (User, error)
	List(ctx context.Context) ([]User, error)
	Count(ctx context.Context) (int64, error)
	// Update writes name, password hash, flags, and disabled_at.
	Update(ctx context.Context, u User) error
}

type Sessions interface {
	Create(ctx context.Context, s Session) error
	GetByHash(ctx context.Context, hash []byte) (Session, error)
	Touch(ctx context.Context, id string, at time.Time) error
	Delete(ctx context.Context, id string) error
	// DeleteForUser deletes every session of the user except exceptID.
	DeleteForUser(ctx context.Context, userID, exceptID string) error
	DeleteExpired(ctx context.Context, now time.Time) error
}

type Projects interface {
	Create(ctx context.Context, p Project) error
	Get(ctx context.Context, id string) (Project, error)
	GetBySlug(ctx context.Context, slug string) (Project, error)
	List(ctx context.Context) ([]Project, error)
	ListForUser(ctx context.Context, userID string) ([]Project, error)
	Update(ctx context.Context, id, name, slug string, at time.Time) error
	UpdateDEK(ctx context.Context, id string, wrapped []byte, rootVersion int64) error
	Delete(ctx context.Context, id string) error

	AddMember(ctx context.Context, m Member) error
	GetMember(ctx context.Context, projectID, userID string) (Member, error)
	ListMembers(ctx context.Context, projectID string) ([]Member, error)
	UpdateMemberRole(ctx context.Context, projectID, userID string, role Role) error
	RemoveMember(ctx context.Context, projectID, userID string) error
	CountOwners(ctx context.Context, projectID string) (int64, error)
}

type Environments interface {
	Create(ctx context.Context, e Environment) error
	Get(ctx context.Context, id string) (Environment, error)
	GetBySlug(ctx context.Context, projectID, slug string) (Environment, error)
	List(ctx context.Context, projectID string) ([]Environment, error)
	// Update writes name, slug, and position.
	Update(ctx context.Context, e Environment) error
	Delete(ctx context.Context, id string) error
}

type Secrets interface {
	Create(ctx context.Context, s Secret) error
	GetLive(ctx context.Context, environmentID, key string) (Secret, error)
	// Update writes key, current version, updated_at, and deleted_at.
	Update(ctx context.Context, s Secret) error
	ListLive(ctx context.Context, environmentID string) ([]LiveSecret, error)
	ListProjectKeys(ctx context.Context, projectID string) ([]SecretKey, error)

	CreateVersion(ctx context.Context, v SecretVersion) error
	GetVersion(ctx context.Context, secretID string, version int64) (SecretVersion, error)
	ListVersions(ctx context.Context, secretID string) ([]SecretVersion, error)
	// PruneVersions deletes versions older than minVersion.
	PruneVersions(ctx context.Context, secretID string, minVersion int64) error
}

type Tokens interface {
	CreateService(ctx context.Context, t ServiceToken) error
	GetService(ctx context.Context, id string) (ServiceToken, error)
	GetServiceByHash(ctx context.Context, hash []byte) (ServiceToken, error)
	ListService(ctx context.Context, projectID string) ([]ServiceToken, error)
	RevokeService(ctx context.Context, id string, at time.Time) error
	TouchService(ctx context.Context, id string, at time.Time) error

	CreatePersonal(ctx context.Context, t PersonalToken) error
	GetPersonalByHash(ctx context.Context, hash []byte) (PersonalToken, error)
	ListPersonal(ctx context.Context, userID string) ([]PersonalToken, error)
	RevokePersonal(ctx context.Context, userID, id string, at time.Time) error
	RevokeAllPersonal(ctx context.Context, userID string, at time.Time) error
	TouchPersonal(ctx context.Context, id string, at time.Time) error
}

type Audit interface {
	Insert(ctx context.Context, l AuditLog) error
	// List returns up to f.Limit rows and the cursor of the next page (nil
	// when there are no more rows).
	List(ctx context.Context, f AuditFilter) ([]AuditLog, *AuditCursor, error)
	DeleteBefore(ctx context.Context, before time.Time) (int64, error)
}

type Meta interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
}

// Store is the data access interface used by the service layer.
type Store interface {
	Users() Users
	Sessions() Sessions
	Projects() Projects
	Environments() Environments
	Secrets() Secrets
	Tokens() Tokens
	Audit() Audit
	Meta() Meta
	// WithTx runs fn in a transaction, committed when fn returns nil.
	// Calling WithTx on the Store passed to fn reuses that transaction.
	WithTx(ctx context.Context, fn func(tx Store) error) error
}

// DB is an open database: a Store plus lifecycle operations.
type DB interface {
	Store
	Ping(ctx context.Context) error
	Close() error
	// Backup writes a consistent snapshot to path (SQLite only).
	Backup(ctx context.Context, path string) error
}

// Millis converts t to unix milliseconds, the storage format for times.
func Millis(t time.Time) int64 { return t.UnixMilli() }

// FromMillis converts unix milliseconds to a UTC time.
func FromMillis(ms int64) time.Time { return time.UnixMilli(ms).UTC() }
