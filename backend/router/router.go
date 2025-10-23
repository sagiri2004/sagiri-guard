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

func NewRouter(httpCtrl *controllers.HTTPController, authCtrl *controllers.AuthController, adminCtrl *controllers.AdminController, deviceCtrl *controllers.DeviceController, cmdCtrl *controllers.CommandController, mw *middleware.Auth) http.Handler {
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

	// device endpoints
	// We need DB inside controller, constructed in initializer
	// We'll attach in initializer by wrapping this router if needed; for simplicity add placeholders here

	return mux
}
