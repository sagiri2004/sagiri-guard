package initialize

import (
	"fmt"
	"net/http"
	"sagiri-guard/backend/app/controllers"
	"sagiri-guard/backend/app/db"
	jwtutil "sagiri-guard/backend/app/jwt"
	"sagiri-guard/backend/app/middleware"
	"sagiri-guard/backend/app/models"
	"sagiri-guard/backend/app/repo"
	"sagiri-guard/backend/app/services"
	"sagiri-guard/backend/config"
	"sagiri-guard/backend/global"
	"sagiri-guard/backend/router"

	"gorm.io/gorm"
)

type App struct {
	Cfg       config.Config
	DB        *gorm.DB
	Router    http.Handler
	HTTP      *controllers.HTTPController
	Auth      *controllers.AuthController
	Admin     *controllers.AdminController
	Devices   *controllers.DeviceController
	Socket    *controllers.SocketController
	Users     *services.UserService
	DeviceSvc *services.DeviceService
}

func Build(configPath string) (*App, error) {
	// Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	global.Config = cfg

	// Connect DB
	gdb, err := db.Connect(db.Config{Host: cfg.DB.Host, Port: cfg.DB.Port, User: cfg.DB.User, Password: cfg.DB.Pass, DBName: cfg.DB.Name})
	if err != nil {
		return nil, fmt.Errorf("connect db: %w", err)
	}
	global.Mdb = gdb

	// Migrate
	if err := gdb.AutoMigrate(&models.User{}, &models.Device{}); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	// Services
	userRepo := repo.NewUserRepository(gdb)
	deviceRepo := repo.NewDeviceRepository(gdb)
	userSvc := services.NewUserService(userRepo)
	deviceSvc := services.NewDeviceService(deviceRepo)
	if err := userSvc.EnsureAdmin("admin", "admin123"); err != nil {
		// non-critical
	}

	// Controllers
	httpCtrl := controllers.NewHTTPController()
	signer := &jwtutil.Signer{Secret: []byte(cfg.JWT.Secret), Issuer: cfg.JWT.Issuer, ExpMin: cfg.JWT.ExpMin}
	authCtrl := controllers.NewAuthController(userSvc, signer)
	socketCtrl := controllers.NewSocketController()
	mw := &middleware.Auth{Signer: signer}
	adminCtrl := controllers.NewAdminController(userSvc)
	deviceCtrl := controllers.NewDeviceController(deviceSvc)

	// Router
	h := router.NewRouter(httpCtrl, authCtrl, adminCtrl, deviceCtrl, mw)
	// Wrap with logging middleware
	h = middleware.Logging(h)

	return &App{Cfg: cfg, DB: gdb, Router: h, HTTP: httpCtrl, Auth: authCtrl, Admin: adminCtrl, Devices: deviceCtrl, Socket: socketCtrl, Users: userSvc, DeviceSvc: deviceSvc}, nil
}
