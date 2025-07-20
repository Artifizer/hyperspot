package sysinfo

import (
	"errors"

	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/core"
	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/libs/utils"
	"gorm.io/gorm"
)

func GetSysInfo() (*utils.SystemInfo, error) {
	return utils.CollectSysInfo()
}

func GetSysInfoPersisted() (*utils.SystemInfo, errorx.Error) {
	sysInfo, err := utils.CollectSysInfo()
	if err != nil {
		return nil, errorx.NewErrInternalServerError("failed to collect sysinfo: %s", err.Error())
	}
	err = SaveSysInfoToDB(sysInfo)
	if err != nil {
		return nil, errorx.NewErrInternalServerError("failed to save sysinfo to db: %s", err.Error())
	}
	return sysInfo, nil
}

func GetSysInfoFromDB(id uuid.UUID) (*utils.SystemInfo, errorx.Error) {
	var sysInfo utils.SystemInfo
	if err := db.DB().Where("id = ?", id).First(&sysInfo).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.NewErrNotFound("sysinfo record not found")
		}
		return nil, errorx.NewErrInternalServerError("failed to get sysinfo from db: %s", err.Error())
	}
	return &sysInfo, nil
}

func SaveSysInfoToDB(sysInfo *utils.SystemInfo) error {
	_, errx := GetSysInfoFromDB(sysInfo.ID)
	if errx == nil {
		return nil
	}
	return db.DB().Save(sysInfo).Error
}

func initMain() error {
	initHardwareSysCaps()
	initOSSysCaps()
	return nil
}

func InitModule() {
	core.RegisterModule(&core.Module{
		Migrations: []interface{}{
			&utils.SystemInfo{},
		},
		InitMain:      initMain,
		InitAPIRoutes: initSysInfoAPIRoutes,
		Name:          "sysinfo",
	})
}
