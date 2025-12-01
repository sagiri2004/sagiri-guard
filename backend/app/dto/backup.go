package dto

type TransferDirection string

const (
	DirectionUpload   TransferDirection = "upload"
	DirectionDownload TransferDirection = "download"
)

type SessionStatus string

const (
	SessionPending   SessionStatus = "pending"
	SessionActive    SessionStatus = "active"
	SessionCompleted SessionStatus = "completed"
	SessionError     SessionStatus = "error"
)

type BackupUploadInitRequest struct {
	FileName    string `json:"file_name"`
	FileSize    int64  `json:"file_size"`
	Checksum    string `json:"checksum,omitempty"`
	LogicalPath string `json:"logical_path,omitempty"`
}

type BackupDownloadInitRequest struct {
	FileName string `json:"file_name"`
}

type BackupSessionResponse struct {
	SessionID string            `json:"session_id"`
	Token     string            `json:"token"`
	FileName  string            `json:"file_name"`
	FileSize  int64             `json:"file_size"`
	Offset    int64             `json:"offset"`
	ChunkSize int64             `json:"chunk_size"`
	TCPHost   string            `json:"tcp_host"`
	TCPPort   int               `json:"tcp_port"`
	Direction TransferDirection `json:"direction"`
	Status    SessionStatus     `json:"status"`
}
