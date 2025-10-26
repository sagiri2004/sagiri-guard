package services

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sagiri-guard/backend/app/dto"
	"sagiri-guard/backend/config"
	"sagiri-guard/backend/global"
	"sagiri-guard/network"
	"sort"
	"strings"
	"time"
)

const (
	// driveURL = "https://graph.microsoft.com/v1.0/me/drive"
	// authURL  = "https://login.microsoftonline.com/common/oauth2/v2.0/token"
	// keep old versions for 30 days by default
	outdatedDuration = 30 * 24 * 60 * 60
)

type BackupService struct {
	cfg *config.Config
}

// StsOnedriveClientService provides helper methods to interact with OneDrive Graph API
// while reusing the same configuration as BackupService.
type StsOnedriveClientService struct {
	cfg *config.Config
}

func NewBackupService() *BackupService {
	return &BackupService{cfg: global.Config}
}

func NewStsOnedriveClientService() *StsOnedriveClientService {
	return &StsOnedriveClientService{cfg: global.Config}
}

func (s *BackupService) AssumeRole(deviceId, mode string) (*dto.OnedriveCredentials, error) {
	accessToken, expiry, err := s.GetAccessTokenFromRefreshToken(mode)
	if err != nil {
		return nil, err
	}
	return &dto.OnedriveCredentials{
		AccessToken:  accessToken,
		Expiry:       expiry,
		DeviceId:     deviceId,
		RootFolderId: s.cfg.Onedrive.RootFolderID,
		Type:         "onedrive",
		DriveType:    s.cfg.Onedrive.DriveType,
		DriveId:      s.cfg.Onedrive.DriveID,
		ClientId:     s.cfg.Onedrive.ClientID,
		ClientSecret: s.cfg.Onedrive.ClientSecret,
	}, nil
}

func (s *BackupService) GetAccessTokenFromRefreshToken(mode string) (string, int64, error) {
	// Validate required config
	if s.cfg.Onedrive.ClientID == "" || s.cfg.Onedrive.ClientSecret == "" || s.cfg.Onedrive.RefreshToken == "" {
		return "", 0, fmt.Errorf("missing onedrive config: client_id/client_secret/refresh_token")
	}

	form := map[string]string{
		"client_id":     s.cfg.Onedrive.ClientID,
		"client_secret": s.cfg.Onedrive.ClientSecret,
		"grant_type":    "refresh_token",
		"scope":         "offline_access Files.ReadWrite.All User.Read",
	}
	if mode == "put" {
		form["refresh_token"] = s.cfg.Onedrive.RefreshToken
	} else {
		form["refresh_token"] = s.cfg.Onedrive.RefreshToken
		form["scope"] = "offline_access Files.Read.All User.Read"
	}

	data := url.Values{}
	for k, v := range form {
		data.Set(k, v)
	}

	respBody, err := network.HTTPPostWithHeaders("login.microsoftonline.com", 443, "/common/oauth2/v2.0/token", "application/x-www-form-urlencoded", []byte(data.Encode()), map[string]string{"Accept": "application/json"})
	if err != nil {
		return "", 0, err
	}

	var result struct {
		AccessToken      string `json:"access_token"`
		Expiry           int64  `json:"expires_in"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal([]byte(respBody), &result); err != nil {
		return "", 0, fmt.Errorf("token decode failed: %w; body=%s", err, respBody)
	}
	if result.AccessToken == "" || result.Expiry == 0 {
		if result.Error != "" {
			return "", 0, fmt.Errorf("token error: %s - %s", result.Error, result.ErrorDescription)
		}
		return "", 0, fmt.Errorf("invalid token response: %s", respBody)
	}

	return result.AccessToken, result.Expiry, nil
}

func (s *StsOnedriveClientService) doRequest(method, host, path, token, contentType string, body []byte) (string, error) {
	headers := map[string]string{}
	if token != "" {
		headers["Authorization"] = fmt.Sprintf("Bearer %s", token)
	}
	switch strings.ToUpper(method) {
	case "GET":
		return network.HTTPGetWithHeaders(host, 443, path, headers)
	case "DELETE":
		return network.HTTPDeleteWithHeaders(host, 443, path, headers)
	case "POST":
		return network.HTTPPostWithHeaders(host, 443, path, contentType, body, headers)
	case "PUT":
		return network.HTTPPutWithHeaders(host, 443, path, contentType, body, headers)
	default:
		return "", fmt.Errorf("unsupported method: %s", method)
	}
}

func (s *StsOnedriveClientService) listFolders(token, path string) ([]dto.ParentFolder, error) {
	var apiPath string
	if path == "" {
		apiPath = "/v1.0/me/drive/root/children"
	} else {
		apiPath = fmt.Sprintf("/v1.0/me/drive/root:/%s:/children", path)
	}
	body, err := s.doRequest("GET", "graph.microsoft.com", apiPath, token, "", nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Value []dto.OnedriveItemsResponse `json:"value"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}

	var folders []dto.ParentFolder
	for _, item := range result.Value {
		if item.Folder != nil {
			folders = append(folders, dto.ParentFolder{
				FolderName: item.Name,
				ItemId:     item.ID,
			})
		}
	}
	return folders, nil
}

func (s *StsOnedriveClientService) listFiles(token, path string) ([]dto.File, error) {
	var apiPath string
	if path == "" {
		apiPath = "/v1.0/me/drive/root/children"
	} else {
		apiPath = fmt.Sprintf("/v1.0/me/drive/root:/%s:/children", path)
	}
	body, err := s.doRequest("GET", "graph.microsoft.com", apiPath, token, "", nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Value []dto.OnedriveItemsResponse `json:"value"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}

	var files []dto.File
	for _, item := range result.Value {
		if item.File != nil {
			files = append(files, item.ToFile())
		}
	}
	return files, nil
}

func (s *StsOnedriveClientService) listVersions(token, itemId string) ([]dto.FileVersion, error) {
	apiPath := fmt.Sprintf("/v1.0/me/drive/items/%s/versions", itemId)
	body, err := s.doRequest("GET", "graph.microsoft.com", apiPath, token, "", nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Value []dto.OnedriveItemVersionResponse `json:"value"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}
	versions := dto.ConvertItemsToDriveVersions(result.Value)
	return versions, nil
}

func (s *StsOnedriveClientService) getFileByDirPath(token, dirPath string) (dto.File, error) {
	apiPath := fmt.Sprintf("/v1.0/me/drive/root:/%s:/children", dirPath)
	body, err := s.doRequest("GET", "graph.microsoft.com", apiPath, token, "", nil)
	if err != nil {
		return dto.File{}, err
	}

	var result struct {
		Value []dto.OnedriveItemsResponse `json:"value"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return dto.File{}, err
	}
	if len(result.Value) == 0 {
		return dto.File{}, fmt.Errorf("no files in path: %s", dirPath)
	}

	var item dto.OnedriveItemsResponse
	for _, it := range result.Value {
		if it.File != nil {
			item = it
			break
		}
	}
	if item.ID == "" {
		return dto.File{}, fmt.Errorf("no file item in path: %s", dirPath)
	}
	parts := strings.Split(dirPath, "/")
	var userId, fileId string
	if len(parts) >= 2 {
		userId = parts[len(parts)-2]
		fileId = parts[len(parts)-1]
	}

	f := item.ToFile()
	return dto.File{
		DeviceId:             userId,
		FileId:               fileId,
		FileName:             f.FileName,
		DriveItemId:          f.DriveItemId,
		LastModifiedDateTime: f.LastModifiedDateTime,
		Size:                 f.Size,
	}, nil
}

func (s *StsOnedriveClientService) GetAllCurrentFiles(token string) ([]dto.File, error) {
	var results []dto.File
	userIds, err := s.listFolders(token, "")
	if err != nil {
		return nil, fmt.Errorf("failed to list device folders: %v", err)
	}

	for _, userId := range userIds {
		fileFolders, err := s.listFolders(token, fmt.Sprintf("%s", userId.FolderName))
		if err != nil {
			continue
		}

		for _, fileId := range fileFolders {
			files, err := s.listFiles(token, fmt.Sprintf("%s/%s", userId.FolderName, fileId.FolderName))
			if err != nil || len(files) == 0 {
				continue
			}

			f := files[0]
			results = append(results, dto.File{
				DeviceId:             userId.FolderName,
				FileId:               fileId.FolderName,
				FileName:             f.FileName,
				DriveItemId:          f.DriveItemId,
				LastModifiedDateTime: f.LastModifiedDateTime,
				ParentItemId:         userId.ItemId,
				Size:                 f.Size,
			})
		}
	}
	return results, nil
}

func (s *StsOnedriveClientService) GetAllCurrentFilesWithVersions(dataFolder, token string) ([]dto.FileWithVersions, error) {
	var results []dto.FileWithVersions
	userIds, err := s.listFolders(token, dataFolder)
	if err != nil {
		return nil, err
	}

	for _, userId := range userIds {
		fileFolders, _ := s.listFolders(token, fmt.Sprintf("%s/%s", dataFolder, userId.FolderName))
		for _, fileId := range fileFolders {
			files, _ := s.listFiles(token, fmt.Sprintf("%s/%s/%s", dataFolder, userId.FolderName, fileId.FolderName))
			if len(files) == 0 {
				continue
			}

			file := files[0]
			versions, _ := s.listVersions(token, file.DriveItemId)

			results = append(results, dto.FileWithVersions{
				DeviceId:     userId.FolderName,
				FileId:       fileId.FolderName,
				FileName:     file.FileName,
				ItemId:       file.DriveItemId,
				ParentItemId: fileId.ItemId,
				Versions:     versions,
			})
		}
	}
	return results, nil
}

func (s *StsOnedriveClientService) GetVersionByFileIdAndVersionId(versionId, fileId, userId, token string) (*dto.FileVersion, error) {
	filePath := fmt.Sprintf("%s/%s", userId, fileId)

	file, err := s.getFileByDirPath(token, filePath)
	if err != nil {
		return nil, err
	}

	versions, err := s.listVersions(token, file.DriveItemId)
	if err != nil {
		return nil, err
	}

	for _, version := range versions {
		if version.VersionId == versionId {
			return &version, nil
		}
	}
	return nil, fmt.Errorf("version not found: %s", versionId)
}

func (s *StsOnedriveClientService) GetVersionByFileId(fileId, userId, token string) ([]dto.FileVersion, error) {
	filePath := fmt.Sprintf("%s/%s", userId, fileId)
	file, err := s.getFileByDirPath(token, filePath)
	if err != nil {
		return nil, err
	}

	versions, err := s.listVersions(token, file.DriveItemId)
	if err != nil {
		return nil, err
	}
	fmt.Println("versions", versions)
	return versions, nil
}

func (s *StsOnedriveClientService) GetAllCurrentFilesAndVersions(token string) ([]dto.FileWithVersions, error) {
	files, err := s.GetAllCurrentFiles(token)
	if err != nil {
		return nil, err
	}

	var filesWithVersions []dto.FileWithVersions
	for _, file := range files {
		versions, err := s.listVersions(token, file.DriveItemId)
		if err != nil {
			return nil, err
		}
		filesWithVersions = append(filesWithVersions, dto.FileWithVersions{
			DeviceId:     file.DeviceId,
			FileId:       file.FileId,
			FileName:     file.FileName,
			ItemId:       file.DriveItemId,
			ParentItemId: file.ParentItemId,
			Versions:     versions,
		})
	}

	return filesWithVersions, nil
}

func (s *StsOnedriveClientService) deleteVersion(versionId, itemId, token string) error {
	apiPath := fmt.Sprintf("/v1.0/me/drive/items/%s/versions/%s", itemId, versionId)
	_, err := s.doRequest("DELETE", "graph.microsoft.com", apiPath, token, "", nil)
	return err
}

func (s *StsOnedriveClientService) DeleteOutdatedVersion(token string) []error {
	files, err := s.GetAllCurrentFilesAndVersions(token)
	if err != nil {
		return []error{fmt.Errorf("failed to list version: %w", err)}
	}
	var mulErr []error
	for _, p := range files {
		if len(p.Versions) == 0 {
			continue
		}

		versions := append([]dto.FileVersion(nil), p.Versions...)
		sort.Slice(versions, func(i, j int) bool {
			if versions[i].LastModifiedDateTime == versions[j].LastModifiedDateTime {
				return versions[i].VersionId < versions[j].VersionId
			}
			return versions[i].LastModifiedDateTime < versions[j].LastModifiedDateTime
		})

		now := time.Now().Unix()
		newest := versions[len(versions)-1]
		if now-newest.LastModifiedDateTime > outdatedDuration {
			if err := s.deleteItem(p.ItemId, token); err != nil {
				mulErr = append(mulErr, fmt.Errorf("failed to delete file: %w", err))
			}

			if err := s.deleteItem(p.ParentItemId, token); err != nil {
				mulErr = append(mulErr, fmt.Errorf("failed to delete parent folder: %w", err))
			}

			continue
		}

		for _, v := range versions[:len(versions)-1] {
			age := now - v.LastModifiedDateTime
			if age > outdatedDuration {
				err = s.deleteVersion(v.VersionId, p.ItemId, token)
				if err != nil {
					mulErr = append(mulErr, fmt.Errorf("failed to delete version: %w", err))
				}
			}
		}
	}
	if len(mulErr) > 0 {
		return mulErr
	}
	return nil
}

func (s *StsOnedriveClientService) deleteItem(itemId, token string) error {
	apiPath := fmt.Sprintf("/v1.0/me/drive/items/%s", itemId)
	_, err := s.doRequest("DELETE", "graph.microsoft.com", apiPath, token, "", nil)
	return err
}
