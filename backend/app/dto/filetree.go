package dto

// FileChange đại diện cho một thay đổi file/folder được agent gửi lên.
type FileChange struct {
	ID             string `json:"id"`
	OriginPath     string `json:"origin_path"`
	CurrentPath    string `json:"cur_path"`
	CurrentName    string `json:"cur_name"`
	Extension      string `json:"cur_ext"`
	Size           int64  `json:"total_size"`
	SnapshotNumber int    `json:"snapshot_number"`
	IsDir          bool   `json:"is_dir"`
	Deleted        bool   `json:"deleted"`
	ChangePending  bool   `json:"change_pending"`
	ContentTypes   []uint `json:"content_type_ids"`
}

// TreeQuery chứa các tham số filter cho API duyệt cây.
type TreeQuery struct {
	DeviceUUID     string
	ParentID       *string
	Extensions     []string
	ContentTypeIDs []uint
	Search         string
	Page           int
	PageSize       int
	IncludeDeleted bool   // Nếu true, sẽ query cả các item đã xóa mềm
	DeletedSince   string // Format: "6h", "24h", "7d" hoặc timestamp. Chỉ áp dụng khi IncludeDeleted=true
}

// ContentTypeAssignment request body để gán nhãn cho node.
type ContentTypeAssignment struct {
	ContentTypeIDs []uint `json:"content_type_ids"`
}

// TreeNodeResponse là DTO trả về cho frontend.
type TreeNodeResponse struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Type           string  `json:"type"`
	ParentID       *string `json:"parent_id,omitempty"`
	Size           int64   `json:"total_size"`
	Extension      string  `json:"extension,omitempty"`
	ContentTypeIDs []uint  `json:"content_type_ids"`
	UpdatedAtUnix  int64   `json:"updated_at"`
	DeletedAtUnix  *int64  `json:"deleted_at,omitempty"` // Timestamp khi item bị xóa mềm, nil nếu chưa xóa
}
