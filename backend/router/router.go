//go:build ignore
// +build ignore

package router

import (
	"fmt"
	"net/http"
	"sagiri-guard/backend/app/controllers"
	"sagiri-guard/backend/app/middleware"
)

type Routes struct {
	Public http.Handler
	Admin  http.Handler
}

func NewRouter(
	httpCtrl *controllers.HTTPController,
	authCtrl *controllers.AuthController,
	adminCtrl *controllers.AdminController,
	deviceCtrl *controllers.DeviceController,
	cmdCtrl *controllers.CommandController,
	backupCtrl *controllers.BackupController,
	agentLogCtrl *controllers.AgentLogController,
	fileTreeCtrl *controllers.FileTreeController,
	contentTypeCtrl *controllers.ContentTypeController,
	adminBackupCtrl *controllers.AdminBackupController,
	websiteBlockCtrl *controllers.WebsiteBlockController,
	mw *middleware.Auth,
) http.Handler {
	mux := http.NewServeMux()
	register := func(pattern string, handler http.Handler) {
		mux.Handle(pattern, middleware.WithRoute(pattern, handler))
	}
	registerFunc := func(pattern string, fn http.HandlerFunc) {
		register(pattern, fn)
	}

	// public
	registerFunc("/ping", httpCtrl.Ping)
	registerFunc("/echo", httpCtrl.Echo)
	registerFunc("/update", httpCtrl.Update)
	registerFunc("/resource", httpCtrl.DeleteResource)
	registerFunc("/upload", httpCtrl.Upload)
	registerFunc("/login", authCtrl.Login)

	// admin-only endpoints
	register("/admin/users", mw.RequireAdmin(http.HandlerFunc(adminCtrl.CreateUser)))

	// command endpoints (admin only)
	register("/admin/command", mw.RequireAdmin(http.HandlerFunc(cmdCtrl.Post)))
	register("/admin/online", mw.RequireAdmin(http.HandlerFunc(cmdCtrl.Online)))
	register("/admin/command/queue", mw.RequireAdmin(http.HandlerFunc(cmdCtrl.Queue)))

	// backup admin (versions)
	if adminBackupCtrl != nil {
		register("/admin/backup/versions", mw.RequireAdmin(http.HandlerFunc(adminBackupCtrl.ListVersions)))
		register("/admin/backup/versions/by-file-id", mw.RequireAdmin(http.HandlerFunc(adminBackupCtrl.ListVersionsByFileID)))
		register("/admin/backup/versions/by-file-id/latest", mw.RequireAdmin(http.HandlerFunc(adminBackupCtrl.GetLatestVersionByFileID)))
		// Restore command is now sent via /admin/command endpoint
	}

	// devices
	register("/devices", mw.RequireAuth(http.HandlerFunc(deviceCtrl.GetByUUID)))
	register("/devices/register", mw.RequireAuth(http.HandlerFunc(deviceCtrl.RegisterOrUpdate)))

	if backupCtrl != nil {
		fmt.Println("registering backup routes")
		backupMux := http.NewServeMux()
		backupMux.Handle("/upload/init", http.HandlerFunc(backupCtrl.InitUpload))
		backupMux.Handle("/download/init", http.HandlerFunc(backupCtrl.InitDownload))
		backupMux.Handle("/session", http.HandlerFunc(backupCtrl.SessionStatus))

		backupHandler := mw.RequireAuth(http.StripPrefix("/backup", backupMux))
		register("/backup", backupHandler)
		register("/backup/", backupHandler)

		register("/backup/upload/init", mw.RequireAuth(http.HandlerFunc(backupCtrl.InitUpload)))
		register("/backup/download/init", mw.RequireAuth(http.HandlerFunc(backupCtrl.InitDownload)))
		register("/backup/session", mw.RequireAuth(http.HandlerFunc(backupCtrl.SessionStatus)))
	} else {
		fmt.Println("backup controller is nil, skipping backup routes")
	}

	// agent logs
	register("/agent/log", mw.RequireAuth(http.HandlerFunc(agentLogCtrl.Post)))
	register("/agent/log/latest", mw.RequireAuth(http.HandlerFunc(agentLogCtrl.GetLatest)))

	// file tree + content types
	register("/filetree/nodes", mw.RequireAuth(http.HandlerFunc(fileTreeCtrl.List)))
	register("/filetree/sync", mw.RequireAuth(http.HandlerFunc(fileTreeCtrl.Sync)))
	register("/filetree/content-types", mw.RequireAuth(http.HandlerFunc(fileTreeCtrl.AssignContentTypes)))

	register("/content-types", mw.RequireAuth(http.HandlerFunc(contentTypeCtrl.List)))
	register("/content-types/create", mw.RequireAdmin(http.HandlerFunc(contentTypeCtrl.Create)))
	register("/content-types/update", mw.RequireAdmin(http.HandlerFunc(contentTypeCtrl.Update)))
	register("/content-types/delete", mw.RequireAdmin(http.HandlerFunc(contentTypeCtrl.Delete)))

	// website blocking (admin only)
	if websiteBlockCtrl != nil {
		register("/admin/website-block/rule", mw.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost:
				websiteBlockCtrl.CreateRule(w, r)
			case http.MethodPut:
				websiteBlockCtrl.UpdateRule(w, r)
			case http.MethodDelete:
				websiteBlockCtrl.DeleteRule(w, r)
			default:
				http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			}
		})))
		register("/admin/website-block/rules", mw.RequireAdmin(http.HandlerFunc(websiteBlockCtrl.ListRules)))
		register("/admin/website-block/status", mw.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				websiteBlockCtrl.GetStatus(w, r)
			} else if r.Method == http.MethodPut {
				websiteBlockCtrl.UpdateStatus(w, r)
			} else {
				http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			}
		})))
		register("/admin/website-block/sync", mw.RequireAdmin(http.HandlerFunc(websiteBlockCtrl.SyncRules)))
	}

	return mux
}
