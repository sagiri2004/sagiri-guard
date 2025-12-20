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
	"sagiri-guard/backend/app/socket"
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
	Backup    *controllers.BackupController
	FileTree  *controllers.FileTreeController
	Content   *controllers.ContentTypeController
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
	if err := gdb.AutoMigrate(
		&models.User{},
		&models.Device{},
		&models.AgentLog{},
		&models.ContentType{},
		&models.FileNode{},
		&models.FolderNode{},
		&models.Item{},
		&models.ItemContentTypeLink{},
		&models.AgentCommand{},
		&models.BackupFileVersion{},
		&models.WebsiteBlockRule{},
		&models.WebsiteBlockStatus{},
	); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	// Services
	userRepo := repo.NewUserRepository(gdb)
	deviceRepo := repo.NewDeviceRepository(gdb)
	agentLogRepo := repo.NewAgentLogRepository(gdb)
	fileTreeRepo := repo.NewFileTreeRepository(gdb)
	contentTypeRepo := repo.NewContentTypeRepository(gdb)
	backupVersionRepo := repo.NewBackupVersionRepository(gdb)
	agentCmdRepo := repo.NewAgentCommandRepository(gdb)
	websiteBlockRepo := repo.NewWebsiteBlockRepository(gdb)

	userSvc := services.NewUserService(userRepo)
	deviceSvc := services.NewDeviceService(deviceRepo)
	agentLogSvc := services.NewAgentLogService(agentLogRepo)
	contentTypeSvc := services.NewContentTypeService(contentTypeRepo)
	fileTreeSvc := services.NewFileTreeService(fileTreeRepo, contentTypeRepo)
	backupSvc, err := services.NewBackupService(cfg, backupVersionRepo)
	if err != nil {
		return nil, fmt.Errorf("init backup service: %w", err)
	}
	websiteBlockSvc := services.NewWebsiteBlockService(websiteBlockRepo)
	if err := userSvc.EnsureAdmin("admin", "admin123"); err != nil {
		// non-critical
	}
	if err := userSvc.EnsureUser("user", "user123", "user"); err != nil {
		// non-critical
	}

	// Controllers
	httpCtrl := controllers.NewHTTPController()
	signer := &jwtutil.Signer{Secret: []byte(cfg.JWT.Secret), Issuer: cfg.JWT.Issuer, ExpMin: cfg.JWT.ExpMin}
	authCtrl := controllers.NewAuthController(userSvc, signer)
	authCtrl.Devices = deviceSvc
	hub := socket.NewHub()
	socketCtrl := controllers.NewSocketController(hub, agentCmdRepo)
	mw := &middleware.Auth{Signer: signer}
	adminCtrl := controllers.NewAdminController(userSvc)
	deviceCtrl := controllers.NewDeviceController(deviceSvc)
	agentLogCtrl := controllers.NewAgentLogController(agentLogSvc)
	backupCtrl := controllers.NewBackupController(backupSvc)
	adminBackupCtrl := controllers.NewAdminBackupController(backupVersionRepo, fileTreeRepo, hub)
	fileTreeCtrl := controllers.NewFileTreeController(fileTreeSvc)
	contentTypeCtrl := controllers.NewContentTypeController(contentTypeSvc)
	websiteBlockCtrl := controllers.NewWebsiteBlockController(websiteBlockSvc, hub, agentCmdRepo)

	// Router
	cmdCtrl := controllers.NewCommandController(hub, agentCmdRepo, fileTreeRepo, backupVersionRepo)
	h := router.NewRouter(httpCtrl, authCtrl, adminCtrl, deviceCtrl, cmdCtrl, backupCtrl, agentLogCtrl, fileTreeCtrl, contentTypeCtrl, adminBackupCtrl, websiteBlockCtrl, mw)
	// Wrap with logging middleware
	h = middleware.Logging(h)

	return &App{
		Cfg:       *cfg,
		DB:        gdb,
		Router:    h,
		HTTP:      httpCtrl,
		Auth:      authCtrl,
		Admin:     adminCtrl,
		Devices:   deviceCtrl,
		Socket:    socketCtrl,
		Users:     userSvc,
		DeviceSvc: deviceSvc,
		Backup:    backupCtrl,
		FileTree:  fileTreeCtrl,
		Content:   contentTypeCtrl,
	}, nil
}
