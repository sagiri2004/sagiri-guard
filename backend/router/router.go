package router

import (
	"net/http"
	"sagiri-guard/backend/app/controllers"
	"sagiri-guard/backend/app/middleware"
)

type Routes struct {
	Public http.Handler
	Admin  http.Handler
}

func NewRouter(httpCtrl *controllers.HTTPController, authCtrl *controllers.AuthController, adminCtrl *controllers.AdminController, deviceCtrl *controllers.DeviceController, cmdCtrl *controllers.CommandController, backupCtrl *controllers.BackupController, agentLogCtrl *controllers.AgentLogController, mw *middleware.Auth) http.Handler {
	mux := http.NewServeMux()
	// public
	mux.HandleFunc("/ping", httpCtrl.Ping)
	mux.HandleFunc("/echo", httpCtrl.Echo)
	mux.HandleFunc("/update", httpCtrl.Update)
	mux.HandleFunc("/resource", httpCtrl.DeleteResource)
	mux.HandleFunc("/upload", httpCtrl.Upload)
	mux.HandleFunc("/login", authCtrl.Login)

	// admin-only endpoints
	adminMux := http.NewServeMux()
	adminMux.HandleFunc("/admin/users", adminCtrl.CreateUser)
	mux.Handle("/admin/users", mw.RequireAdmin(adminMux))

	// command endpoints (admin only)
	mux.Handle("/admin/command", mw.RequireAdmin(http.HandlerFunc(cmdCtrl.Post)))
	mux.Handle("/admin/online", mw.RequireAdmin(http.HandlerFunc(cmdCtrl.Online)))

	// devices
	mux.Handle("/devices", mw.RequireAuth(http.HandlerFunc(deviceCtrl.GetByUUID)))
	mux.Handle("/devices/register", mw.RequireAuth(http.HandlerFunc(deviceCtrl.RegisterOrUpdate)))

	// backup onedrive (auth required)
	if backupCtrl != nil {
		mux.Handle("/backup/onedrive/credential", mw.RequireAuth(http.HandlerFunc(backupCtrl.GetUploadCredential)))
		mux.Handle("/backup/onedrive/files", mw.RequireAuth(http.HandlerFunc(backupCtrl.GetAllCurrentFiles)))
		mux.Handle("/backup/onedrive/file/versions", mw.RequireAuth(http.HandlerFunc(backupCtrl.GetVersionByFileId)))
		mux.Handle("/backup/onedrive/file/version", mw.RequireAuth(http.HandlerFunc(backupCtrl.GetVersionByFileIdAndVersionId)))
		mux.Handle("/backup/onedrive/files-versions", mw.RequireAuth(http.HandlerFunc(backupCtrl.GetAllCurrentFilesAndVersions)))
	}

	// agent logs
	mux.Handle("/agent/log", mw.RequireAuth(http.HandlerFunc(agentLogCtrl.Post)))
	mux.Handle("/agent/log/latest", mw.RequireAuth(http.HandlerFunc(agentLogCtrl.GetLatest)))

	return mux
}
