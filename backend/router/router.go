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

func NewRouter(httpCtrl *controllers.HTTPController, authCtrl *controllers.AuthController, adminCtrl *controllers.AdminController, deviceCtrl *controllers.DeviceController, cmdCtrl *controllers.CommandController, backupCtrl *controllers.BackupController, agentLogCtrl *controllers.AgentLogController, mw *middleware.Auth) http.Handler {
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

	// devices
	register("/devices", mw.RequireAuth(http.HandlerFunc(deviceCtrl.GetByUUID)))
	register("/devices/register", mw.RequireAuth(http.HandlerFunc(deviceCtrl.RegisterOrUpdate)))

	// backup onedrive (auth required)
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

	return mux
}
