package dto

import (
	"time"
)

type OnedriveCredentials struct {
	AccessToken  string `json:"access_token"`
	DeviceId     string `json:"device_id"`
	RootFolderId string `json:"root_folder_id"`
	Type         string `json:"type"`
	DriveType    string `json:"drive_type"`
	DriveId      string `json:"drive_id"`
	ClientId     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Expiry       int64  `json:"expiry"`
}

type OnedriveItemVersionResponse struct {
	ID                   string `json:"id"`
	Size                 int64  `json:"size"`
	LastModifiedDateTime string `json:"lastModifiedDateTime"`
	DownloadURL          string `json:"@microsoft.graph.downloadUrl"`
}

type OnedriveItemsResponse struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Size                 int64  `json:"size"`
	LastModifiedDateTime string `json:"lastModifiedDateTime"`
	CreatedDateTime      string `json:"createdDateTime"`
	File                 *struct {
		MimeType string `json:"mimeType"`
	} `json:"file,omitempty"`
	Folder *struct {
		ChildCount int `json:"childCount"`
	} `json:"folder,omitempty"`
}

type FileVersion struct {
	VersionId            string `json:"version_id"`
	Size                 int64  `json:"size"`
	LastModifiedDateTime int64  `json:"last_modified_date_time"`
	DownloadURL          string `json:"download_url"`
}

type File struct {
	DeviceId             string `json:"device_id"`
	FileId               string `json:"file_id"`
	FileName             string `json:"file_name"`
	DriveItemId          string `json:"drive_item_id"`
	LastModifiedDateTime int64  `json:"last_modified_date_time"`
	Size                 int64  `json:"size"`
	ParentItemId         string `json:"parent_item_id"`
}

type FileWithVersions struct {
	DeviceId     string        `json:"device_id"`
	FileId       string        `json:"file_id"`
	FileName     string        `json:"file_name"`
	ItemId       string        `json:"item_id"`
	ParentItemId string        `json:"parent_item_id"`
	Versions     []FileVersion `json:"versions"`
}

type ParentFolder struct {
	FolderName string `json:"folder_name"`
	ItemId     string `json:"item_id"`
}

func (v OnedriveItemVersionResponse) ToFileVersion() FileVersion {
	return FileVersion{
		VersionId:            v.ID,
		Size:                 v.Size,
		LastModifiedDateTime: parseGraphTimeToUnix(v.LastModifiedDateTime),
		DownloadURL:          v.DownloadURL,
	}
}

func (i OnedriveItemsResponse) ToFile() File {
	return File{
		FileName:             i.Name,
		DriveItemId:          i.ID,
		LastModifiedDateTime: parseGraphTimeToUnix(i.LastModifiedDateTime),
		Size:                 i.Size,
	}
}

func ConvertItemsToDriveFiles(items []OnedriveItemsResponse) []File {
	files := make([]File, 0, len(items))
	for _, item := range items {
		files = append(files, item.ToFile())
	}
	return files
}

func ConvertItemsToDriveVersions(items []OnedriveItemVersionResponse) []FileVersion {
	versions := make([]FileVersion, 0, len(items))
	for _, it := range items {
		if it.DownloadURL == "" {
			continue
		}
		versions = append(versions, it.ToFileVersion())
	}
	return versions
}

func parseGraphTimeToUnix(s string) int64 {
	if s == "" {
		return 0
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Unix()
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.Unix()
	}
	return 0
}
