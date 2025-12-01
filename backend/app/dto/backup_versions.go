package dto

// BackupVersionResponse là DTO trả về cho admin khi liệt kê các version.
type BackupVersionResponse struct {
	ID          uint   `json:"id"`
	DeviceID    string `json:"device_id"`
	LogicalPath string `json:"logical_path"`
	FileName    string `json:"file_name"`
	StoredName  string `json:"stored_name"`
	Version     int    `json:"version"`
	Size        int64  `json:"size"`
	CreatedAt   int64  `json:"created_at"`
}

// BackupRestoreRequest là body request khi admin yêu cầu restore.
type BackupRestoreRequest struct {
	DeviceID    string `json:"device_id"`
	LogicalPath string `json:"logical_path"`
	Version     int    `json:"version,omitempty"`   // 0 hoặc bỏ trống = latest
	DestPath    string `json:"dest_path,omitempty"` // nếu rỗng, agent sẽ chọn default
}


