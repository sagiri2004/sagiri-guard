package services

import (
	"path/filepath"
	"strings"
	"time"

	"sagiri-guard/backend/app/dto"
	"sagiri-guard/backend/app/models"
	"sagiri-guard/backend/app/repo"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type FileTreeService struct {
	treeRepo    *repo.FileTreeRepository
	contentRepo *repo.ContentTypeRepository
}

func NewFileTreeService(treeRepo *repo.FileTreeRepository, contentRepo *repo.ContentTypeRepository) *FileTreeService {
	return &FileTreeService{
		treeRepo:    treeRepo,
		contentRepo: contentRepo,
	}
}

func (s *FileTreeService) ApplyChanges(deviceUUID string, changes []dto.FileChange) error {
	for _, change := range changes {
		if change.Deleted {
			if err := s.treeRepo.DeleteItem(deviceUUID, change.ID); err != nil {
				return err
			}
			continue
		}
		if _, err := s.upsertPath(deviceUUID, change); err != nil {
			return err
		}
	}
	return nil
}

func (s *FileTreeService) GetNodes(query dto.TreeQuery) ([]*models.Item, int64, error) {
	filter := repo.ItemFilter{
		DeviceUUID:     query.DeviceUUID,
		ParentID:       query.ParentID,
		Search:         query.Search,
		Extensions:     query.Extensions,
		ContentTypeIDs: query.ContentTypeIDs,
		Page:           query.Page,
		PageSize:       query.PageSize,
	}
	return s.treeRepo.ListItems(filter)
}

func (s *FileTreeService) AssignContentTypes(deviceUUID, itemID string, contentTypeIDs []uint) error {
	item, err := s.treeRepo.GetItemByID(deviceUUID, itemID)
	if err != nil {
		return err
	}
	if item == nil {
		return gorm.ErrRecordNotFound
	}
	if len(contentTypeIDs) > 0 {
		for _, id := range contentTypeIDs {
			ct, err := s.contentRepo.Get(id)
			if err != nil {
				return err
			}
			if ct == nil {
				return gorm.ErrRecordNotFound
			}
		}
	}
	return s.treeRepo.ReplaceContentTypes(item.ID, contentTypeIDs)
}

func (s *FileTreeService) upsertPath(deviceUUID string, change dto.FileChange) (*models.Item, error) {
	segments, name := splitPath(change.CurrentPath, change.CurrentName)
	if len(segments) == 0 {
		segments = append(segments, name)
	}

	var parentID *string
	var lastItem *models.Item

	for idx, segment := range segments {
		isLast := idx == len(segments)-1
		if isLast && !change.IsDir {
			itemID := change.ID
			if itemID == "" {
				itemID = deterministicID(deviceUUID, segments)
			}
			fileNode := &models.FileNode{
				ID:             itemID,
				DeviceUUID:     deviceUUID,
				OriginPath:     fallback(change.OriginPath, change.CurrentPath),
				CurrentPath:    change.CurrentPath,
				CurrentName:    change.CurrentName,
				Extension:      change.Extension,
				Size:           change.Size,
				SnapshotNumber: change.SnapshotNumber,
				ChangePending:  change.ChangePending,
				LastEventAt:    time.Now(),
			}
			if err := s.treeRepo.UpsertFile(fileNode); err != nil {
				return nil, err
			}
			item := &models.Item{
				ID:         itemID,
				DeviceUUID: deviceUUID,
				Name:       change.CurrentName,
				ParentID:   parentID,
				FileID:     &fileNode.ID,
				TotalSize:  change.Size,
			}
			if err := s.treeRepo.UpsertItem(item); err != nil {
				return nil, err
			}
			if len(change.ContentTypes) > 0 {
				if err := s.treeRepo.ReplaceContentTypes(item.ID, change.ContentTypes); err != nil {
					return nil, err
				}
			}
			lastItem = item
		} else {
			folderID := deterministicID(deviceUUID, segments[:idx+1])
			folderNode := &models.FolderNode{
				ID:         folderID,
				DeviceUUID: deviceUUID,
			}
			if err := s.treeRepo.UpsertFolder(folderNode); err != nil {
				return nil, err
			}
			item := &models.Item{
				ID:         folderID,
				DeviceUUID: deviceUUID,
				Name:       segment,
				ParentID:   parentID,
				FolderID:   &folderNode.ID,
			}
			if err := s.treeRepo.UpsertItem(item); err != nil {
				return nil, err
			}
			parentID = &item.ID
			lastItem = item
		}
	}
	return lastItem, nil
}

func splitPath(curPath, fallbackName string) ([]string, string) {
	if curPath == "" {
		return []string{fallbackName}, fallbackName
	}
	normalized := filepath.ToSlash(curPath)
	parts := strings.Split(normalized, "/")
	var segments []string
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			segments = append(segments, trimmed)
		}
	}
	if len(segments) == 0 {
		return []string{fallbackName}, fallbackName
	}
	return segments, segments[len(segments)-1]
}

func deterministicID(deviceUUID string, segments []string) string {
	key := strings.Join(segments, "/")
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(deviceUUID+"|"+key)).String()
}

func fallback(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
