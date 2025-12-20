package initialize

import (
	"fmt"
	"sagiri-guard/backend/app/controllers"
	"sagiri-guard/backend/app/db"
	jwtutil "sagiri-guard/backend/app/jwt"
	"sagiri-guard/backend/app/models"
	"sagiri-guard/backend/app/repo"
	"sagiri-guard/backend/app/services"
	"sagiri-guard/backend/app/socket"
	"sagiri-guard/backend/config"
	"sagiri-guard/backend/global"

	"gorm.io/gorm"
)

type App struct {
	Cfg       config.Config
	DB        *gorm.DB
	Protocol  *controllers.ProtocolController
	DeviceSvc *services.DeviceService
	FileTree  *services.FileTreeService
	BackupSvc *services.BackupService
	LogsSvc   *services.AgentLogService
	UserSvc   *services.UserService
	Signer    *jwtutil.Signer
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

	// Services / repos needed for protocol
	userRepo := repo.NewUserRepository(gdb)
	deviceRepo := repo.NewDeviceRepository(gdb)
	agentLogRepo := repo.NewAgentLogRepository(gdb)
	fileTreeRepo := repo.NewFileTreeRepository(gdb)
	backupVersionRepo := repo.NewBackupVersionRepository(gdb)
	agentCmdRepo := repo.NewAgentCommandRepository(gdb)

	userSvc := services.NewUserService(userRepo)
	deviceSvc := services.NewDeviceService(deviceRepo)
	agentLogSvc := services.NewAgentLogService(agentLogRepo)
	fileTreeSvc := services.NewFileTreeService(fileTreeRepo, repo.NewContentTypeRepository(gdb))
	backupSvc, err := services.NewBackupService(cfg, backupVersionRepo)
	if err != nil {
		return nil, fmt.Errorf("init backup service: %w", err)
	}

	// Ensure default users (non-critical)
	_ = userSvc.EnsureAdmin("admin", "admin123")
	_ = userSvc.EnsureUser("user", "user123", "user")

	signer := &jwtutil.Signer{
		Secret: []byte(cfg.JWT.Secret),
		Issuer: cfg.JWT.Issuer,
		ExpMin: cfg.JWT.ExpMin,
	}

	hub := socket.NewHub()
	protocolCtrl := controllers.NewProtocolController(hub, agentCmdRepo, deviceSvc, fileTreeSvc, agentLogSvc, backupSvc, userSvc, signer)

	return &App{
		Cfg:       *cfg,
		DB:        gdb,
		DeviceSvc: deviceSvc,
		FileTree:  fileTreeSvc,
		BackupSvc: backupSvc,
		LogsSvc:   agentLogSvc,
		UserSvc:   userSvc,
		Signer:    signer,
		Protocol:  protocolCtrl,
	}, nil
}
