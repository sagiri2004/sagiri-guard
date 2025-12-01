package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sagiri-guard/backend/app/dto"
	"sagiri-guard/backend/app/models"
	"sagiri-guard/backend/app/repo"
	"sagiri-guard/backend/config"
	"sync"
	"time"
)

var (
	ErrSessionNotFound   = errors.New("backup session not found")
	ErrInvalidSession    = errors.New("invalid backup session")
	ErrDirectionMismatch = errors.New("direction mismatch")
)

type BackupSession struct {
	ID        string
	Token     string
	DeviceID  string
	LogicalPath string
	FileName  string
	FileSize  int64
	Checksum  string
	Direction dto.TransferDirection
	Status    dto.SessionStatus
	TempPath  string
	FinalPath string
	BytesDone int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

type BackupService struct {
	storageDir string
	chunkSize  int64
	tcpHost    string
	tcpPort    int
	sessions   map[string]*BackupSession
	mu         sync.RWMutex
	versions   *repo.BackupVersionRepository
}

func NewBackupService(cfg *config.Config, versions *repo.BackupVersionRepository) (*BackupService, error) {
	storage := cfg.Backup.StoragePath
	if storage == "" {
		storage = "backups"
	}
	if err := os.MkdirAll(storage, 0o755); err != nil {
		return nil, fmt.Errorf("create backup dir: %w", err)
	}
	chunkSize := cfg.Backup.ChunkSize
	if chunkSize <= 0 {
		chunkSize = 512 * 1024
	}
	host := cfg.Backup.TCP.Host
	if host == "" {
		host = cfg.TCP.Host
	}
	port := cfg.Backup.TCP.Port
	if port <= 0 {
		port = cfg.TCP.Port + 1
	}
	return &BackupService{
		storageDir: storage,
		chunkSize:  chunkSize,
		tcpHost:    host,
		tcpPort:    port,
		sessions:   make(map[string]*BackupSession),
		versions:   versions,
	}, nil
}

func (s *BackupService) ChunkSize() int64 { return s.chunkSize }
func (s *BackupService) TCPHost() string  { return s.tcpHost }
func (s *BackupService) TCPPort() int     { return s.tcpPort }

func (s *BackupService) PrepareUpload(deviceID string, req dto.BackupUploadInitRequest) (*dto.BackupSessionResponse, error) {
	if deviceID == "" {
		return nil, errors.New("missing device id")
	}
	if req.FileName == "" || req.FileSize <= 0 {
		return nil, errors.New("invalid file metadata")
	}
	safeName := filepath.Base(req.FileName)
	logicalPath := req.LogicalPath
	if logicalPath == "" {
		logicalPath = safeName
	}
	storedName := fmt.Sprintf("%d_%s", time.Now().Unix(), safeName)
	finalPath := filepath.Join(s.storageDir, deviceID, storedName)
	tempPath := finalPath + ".part"
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir device dir: %w", err)
	}
	var offset int64
	if info, err := os.Stat(tempPath); err == nil {
		offset = info.Size()
	} else if errors.Is(err, os.ErrNotExist) {
		f, createErr := os.Create(tempPath)
		if createErr != nil {
			return nil, fmt.Errorf("create temp file: %w", createErr)
		}
		_ = f.Close()
	} else {
		return nil, fmt.Errorf("stat temp file: %w", err)
	}
	if offset > req.FileSize && req.FileSize > 0 {
		offset = req.FileSize
	}
	session := &BackupSession{
		ID:          newID("up"),
		Token:       newToken(),
		DeviceID:    deviceID,
		LogicalPath: logicalPath,
		FileName:    safeName,
		FileSize:    req.FileSize,
		Checksum:    req.Checksum,
		Direction:   dto.DirectionUpload,
		Status:      dto.SessionActive,
		TempPath:    tempPath,
		FinalPath:   finalPath,
		BytesDone:   offset,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	s.mu.Lock()
	s.sessions[session.ID] = session
	s.mu.Unlock()
	return s.toResponse(session), nil
}

func (s *BackupService) PrepareDownload(deviceID string, req dto.BackupDownloadInitRequest) (*dto.BackupSessionResponse, error) {
	if deviceID == "" || req.FileName == "" {
		return nil, errors.New("missing download parameters")
	}
	safeName := filepath.Base(req.FileName)
	finalPath := filepath.Join(s.storageDir, deviceID, safeName)
	info, err := os.Stat(finalPath)
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}
	session := &BackupSession{
		ID:        newID("down"),
		Token:     newToken(),
		DeviceID:  deviceID,
		FileName:  safeName,
		FileSize:  info.Size(),
		Direction: dto.DirectionDownload,
		Status:    dto.SessionActive,
		FinalPath: finalPath,
		BytesDone: 0,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	s.mu.Lock()
	s.sessions[session.ID] = session
	s.mu.Unlock()
	return s.toResponse(session), nil
}

func (s *BackupService) GetSession(id string) (*dto.BackupSessionResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if sess, ok := s.sessions[id]; ok {
		return s.toResponse(sess), nil
	}
	return nil, ErrSessionNotFound
}

func (s *BackupService) ValidateSession(id, token string, direction dto.TransferDirection) (*BackupSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	if sess.Token != token {
		return nil, ErrInvalidSession
	}
	if sess.Direction != direction {
		return nil, ErrDirectionMismatch
	}
	return sess, nil
}

func (s *BackupService) Advance(id string, delta int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return 0, ErrSessionNotFound
	}
	sess.BytesDone += delta
	sess.UpdatedAt = time.Now()
	return sess.BytesDone, nil
}

func (s *BackupService) CurrentOffset(id string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	if !ok {
		return 0, ErrSessionNotFound
	}
	return sess.BytesDone, nil
}

func (s *BackupService) MarkCompleted(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return ErrSessionNotFound
	}
	sess.Status = dto.SessionCompleted
	sess.BytesDone = sess.FileSize
	sess.UpdatedAt = time.Now()
	return nil
}

func (s *BackupService) FinalizeUpload(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return ErrSessionNotFound
	}
	if sess.Direction != dto.DirectionUpload {
		return ErrDirectionMismatch
	}
	if sess.TempPath == "" {
		return errors.New("missing temp path")
	}
	if err := os.Rename(sess.TempPath, sess.FinalPath); err != nil {
		return fmt.Errorf("finalize upload: %w", err)
	}
	sess.Status = dto.SessionCompleted
	sess.BytesDone = sess.FileSize
	sess.UpdatedAt = time.Now()

	// Ghi lại version mới cho file này
	if s.versions != nil && sess.LogicalPath != "" {
		nextVer, err := s.versions.NextVersion(sess.DeviceID, sess.LogicalPath)
		if err != nil {
			return fmt.Errorf("compute next version: %w", err)
		}
		v := &models.BackupFileVersion{
			DeviceID:    sess.DeviceID,
			LogicalPath: sess.LogicalPath,
			FileName:    sess.FileName,
			StoredName:  filepath.Base(sess.FinalPath),
			Version:     nextVer,
			Size:        sess.FileSize,
		}
		if err := s.versions.Create(v); err != nil {
			return fmt.Errorf("store backup version: %w", err)
		}
	}

	return nil
}

func (s *BackupService) toResponse(session *BackupSession) *dto.BackupSessionResponse {
	return &dto.BackupSessionResponse{
		SessionID: session.ID,
		Token:     session.Token,
		FileName:  session.FileName,
		FileSize:  session.FileSize,
		Offset:    session.BytesDone,
		ChunkSize: s.chunkSize,
		TCPHost:   s.tcpHost,
		TCPPort:   s.tcpPort,
		Direction: session.Direction,
		Status:    session.Status,
	}
}

func newID(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, newToken())
}

func newToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
